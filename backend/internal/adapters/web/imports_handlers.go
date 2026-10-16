package web

import (
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

type ImportHandler struct {
	profiles       ports.BankProfileRepository
	importCsv      *services.ImportCsvUseCase
	autoMatch      *services.ReconcileUseCase
	autoCategorize *services.AutoCategorizeUseCase
	accounts       *AccountHandler
}

func NewImportHandler(
	profiles ports.BankProfileRepository,
	importCsv *services.ImportCsvUseCase,
	autoMatch *services.ReconcileUseCase,
	autoCategorize *services.AutoCategorizeUseCase,
	accounts *AccountHandler,
) *ImportHandler {
	return &ImportHandler{profiles: profiles, importCsv: importCsv, autoMatch: autoMatch, autoCategorize: autoCategorize, accounts: accounts}
}

type importBatchResponse struct {
	ID             string    `json:"id"`
	Filename       string    `json:"filename"`
	ImportedAt     time.Time `json:"imported_at"`
	RowCount       int       `json:"row_count"`
	ImportedCount  int       `json:"imported_count"`
	DuplicateCount int       `json:"duplicate_count"`
	Errors         []string  `json:"errors"`
}

func toImportBatchResponse(b entities.ImportBatch) importBatchResponse {
	errs := b.Errors
	if errs == nil {
		errs = []string{}
	}
	return importBatchResponse{
		ID: b.ID.String(), Filename: b.Filename, ImportedAt: b.ImportedAt, RowCount: b.RowCount,
		ImportedCount: b.ImportedCount, DuplicateCount: b.DuplicateCount, Errors: errs,
	}
}

// Import handles multipart/form-data with a "bank_profile_id" field and a
// "file" upload — mirrors POST /accounts/{account_id}/imports in imports.py.
func (h *ImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.RequireOwnAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid multipart form"))
		return
	}
	profileID, err := uuid.Parse(r.FormValue("bank_profile_id"))
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid bank_profile_id"))
		return
	}
	profile, err := h.profiles.GetByID(r.Context(), profileID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if profile == nil || (profile.UserID != nil && *profile.UserID != CurrentUser(r).ID) {
		httpx.WriteError(w, httpx.NotFound("bank profile not found"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("file is required"))
		return
	}
	defer file.Close()
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	filename := "upload.csv"
	if header != nil && header.Filename != "" {
		filename = header.Filename
	}

	batch, err := h.importCsv.Execute(r.Context(), account.ID, *profile, filename, fileBytes)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Newly imported operations whose label matches an existing recurring
	// item get reconciled (and categorized) right away — see
	// ReconcileUseCase.AutoMatchByLabel.
	if _, err := h.autoMatch.AutoMatchByLabel(r.Context(), account.ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Whatever's still uncategorized after that gets a best-effort category
	// from the merchant dictionary / exact-label memory — see
	// AutoCategorizeUseCase.
	if _, err := h.autoCategorize.CategorizeAccount(r.Context(), account.ID, account.UserID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toImportBatchResponse(*batch))
}
