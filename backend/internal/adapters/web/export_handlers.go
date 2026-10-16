package web

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

// ExportHandler serves the RGPD data export (right of access / portability):
// one JSON document with everything the signed-in user owns.
type ExportHandler struct {
	export *services.ExportUserDataUseCase
	// limiter keeps the export (a full-table read) from being hammered: 3 at
	// once per user, then one every 10 minutes.
	limiter *rateLimiter
}

func NewExportHandler(export *services.ExportUserDataUseCase) *ExportHandler {
	return &ExportHandler{export: export, limiter: newRateLimiter(10*time.Minute, 3)}
}

type exportUser struct {
	ID            string    `json:"id"`
	Email         *string   `json:"email"`
	DisplayName   string    `json:"display_name"`
	CreatedAt     time.Time `json:"created_at"`
	EmailVerified bool      `json:"email_verified"`
	HasPassword   bool      `json:"has_password"`
	LinkedGoogle  bool      `json:"linked_google"`
	LinkedApple   bool      `json:"linked_apple"`
	LinkedRobine  bool      `json:"linked_robine"`
}

type exportItem struct {
	plannedItemResponse
	Occurrences []occurrenceResponse `json:"occurrences"`
}

type exportAccount struct {
	accountResponse
	BalanceCheckpoints []balanceCheckpointResponse `json:"balance_checkpoints"`
	Operations         []operationResponse         `json:"operations"`
	RecurringItems     []exportItem                `json:"recurring_items"`
	Imports            []importBatchResponse       `json:"imports"`
}

type exportCollaborator struct {
	InvitedEmail string     `json:"invited_email"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	AcceptedAt   *time.Time `json:"accepted_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
}

type exportBankConnection struct {
	Connected bool   `json:"connected"`
	Bank      string `json:"bank,omitempty"`
	Note      string `json:"note"`
}

type exportDocument struct {
	ExportedAt    time.Time             `json:"exported_at"`
	Format        string                `json:"format"`
	User          exportUser            `json:"user"`
	Accounts      []exportAccount       `json:"accounts"`
	Categories    []categoryResponse    `json:"categories"`
	BankProfiles  []bankProfileResponse `json:"bank_profiles"`
	Comments      []commentResponse     `json:"comments"`
	Collaborators []exportCollaborator  `json:"collaborators"`
	BankSync      exportBankConnection  `json:"bank_sync"`
}

func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	if ok, retry := h.limiter.take(user.ID.String()); !ok {
		writeTooManyRequests(w, retry)
		return
	}
	data, err := h.export.Execute(r.Context(), *user)
	if err != nil {
		log.Printf("export: user %s: %v", user.ID, err)
		httpx.WriteError(w, err)
		return
	}
	doc := toExportDocument(data, time.Now().UTC())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="lagune-export.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		log.Printf("export: user %s: encode: %v", user.ID, err)
	}
}

func toExportDocument(d *services.UserExport, now time.Time) exportDocument {
	u := d.User
	doc := exportDocument{
		ExportedAt: now,
		Format:     "lagune-export/1",
		User: exportUser{
			ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt,
			EmailVerified: u.EmailVerified(), HasPassword: u.PasswordHash != nil,
			LinkedGoogle: u.GoogleSub != nil, LinkedApple: u.AppleSub != nil, LinkedRobine: u.RobineSub != nil,
		},
		// Empty slices, not null, so the document shape is stable.
		Accounts: []exportAccount{}, Categories: []categoryResponse{}, BankProfiles: []bankProfileResponse{},
		Comments: []commentResponse{}, Collaborators: []exportCollaborator{},
		BankSync: exportBankConnection{
			Connected: d.BankConnected,
			Note:      "Identifiants et session bancaires sont chiffrés côté serveur et volontairement exclus de cet export.",
		},
	}
	if d.BankConnected {
		doc.BankSync.Bank = string(d.BankName)
	}
	for _, a := range d.Accounts {
		ea := exportAccount{
			accountResponse:    accountResponse{ID: a.Account.ID.String(), Name: a.Account.Name, Currency: string(a.Account.Currency), CreatedAt: a.Account.CreatedAt},
			BalanceCheckpoints: []balanceCheckpointResponse{}, Operations: []operationResponse{},
			RecurringItems: []exportItem{}, Imports: []importBatchResponse{},
		}
		for _, c := range a.Checkpoints {
			ea.BalanceCheckpoints = append(ea.BalanceCheckpoints, toCheckpointResponse(c))
		}
		for _, op := range a.Operations {
			ea.Operations = append(ea.Operations, toOperationResponse(op, a.Splits[op.ID]))
		}
		for _, it := range a.Items {
			ei := exportItem{plannedItemResponse: toPlannedItemResponse(it.Item), Occurrences: []occurrenceResponse{}}
			for _, o := range it.Occurrences {
				ei.Occurrences = append(ei.Occurrences, toOccurrenceResponse(o, nil))
			}
			ea.RecurringItems = append(ea.RecurringItems, ei)
		}
		for _, b := range a.Imports {
			ea.Imports = append(ea.Imports, toImportBatchResponse(b))
		}
		doc.Accounts = append(doc.Accounts, ea)
	}
	for _, c := range d.Categories {
		doc.Categories = append(doc.Categories, toCategoryResponse(c, u.ID))
	}
	for _, p := range d.BankProfiles {
		doc.BankProfiles = append(doc.BankProfiles, toBankProfileResponse(p))
	}
	for _, c := range d.Comments {
		doc.Comments = append(doc.Comments, toCommentResponse(c))
	}
	for _, c := range d.Collaborators {
		doc.Collaborators = append(doc.Collaborators, toExportCollaborator(c))
	}
	return doc
}

func toExportCollaborator(c entities.Collaborator) exportCollaborator {
	// The invite token is a bearer secret: deliberately not exported.
	return exportCollaborator{
		InvitedEmail: c.InvitedEmail, Role: string(c.Role), CreatedAt: c.CreatedAt,
		AcceptedAt: c.AcceptedAt, RevokedAt: c.RevokedAt,
	}
}
