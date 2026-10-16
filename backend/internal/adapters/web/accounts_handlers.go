package web

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/httpx"
	"bailleul.dev/budget-server/pkg/money"
)

type AccountHandler struct {
	accounts      ports.AccountRepository
	users         ports.UserRepository
	collaborators ports.CollaboratorRepository
}

func NewAccountHandler(accounts ports.AccountRepository, users ports.UserRepository, collaborators ports.CollaboratorRepository) *AccountHandler {
	return &AccountHandler{accounts: accounts, users: users, collaborators: collaborators}
}

type accountResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
	// OwnerDisplayName is set only when this account belongs to someone who
	// shared it with the current viewer — nil means it's the viewer's own.
	OwnerDisplayName *string `json:"owner_display_name"`
}

func (h *AccountHandler) toAccountResponse(ctx context.Context, a entities.Account, viewerID uuid.UUID) accountResponse {
	resp := accountResponse{ID: a.ID.String(), Name: a.Name, Currency: string(a.Currency), CreatedAt: a.CreatedAt}
	if a.UserID != viewerID {
		if owner, err := h.users.GetByID(ctx, a.UserID); err == nil && owner != nil {
			resp.OwnerDisplayName = &owner.DisplayName
		}
	}
	return resp
}

// ResolveOwnerIDs returns viewerID plus the owner_user_id of every account
// currently shared with viewerID (an accepted, non-revoked collaborator
// grant) — every listing that should include a collaborator's shared
// accounts alongside their own loops over this instead of just viewerID.
func (h *AccountHandler) ResolveOwnerIDs(ctx context.Context, viewerID uuid.UUID) ([]uuid.UUID, error) {
	active, err := h.collaborators.ListActiveForCollaborator(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(active)+1)
	ids = append(ids, viewerID)
	for _, c := range active {
		ids = append(ids, c.OwnerUserID)
	}
	return ids, nil
}

// canAccessOwner reports whether viewerID may access ownerID's accounts —
// either they're the same person, or ownerID has an active collaborator
// grant to viewerID.
func (h *AccountHandler) canAccessOwner(ctx context.Context, viewerID, ownerID uuid.UUID) (bool, error) {
	if viewerID == ownerID {
		return true, nil
	}
	active, err := h.collaborators.ListActiveForCollaborator(ctx, viewerID)
	if err != nil {
		return false, err
	}
	for _, c := range active {
		if c.OwnerUserID == ownerID {
			return true, nil
		}
	}
	return false, nil
}

// AccessibleAccount loads the account behind {accountID} if the current
// user owns it or has an active collaborator grant from its owner — a
// missing account and one outside the visitor's reach both 404, mirroring
// the old get_owned_account behavior of never leaking existence. Use this
// (not RequireOwnAccount) for anything a collaborator should be able to
// view or act on — see entities.CollaboratorRoleCollaborator's doc comment
// for the exact scope.
func (h *AccountHandler) AccessibleAccount(r *http.Request) (*entities.Account, error) {
	id, err := parseUUIDParam(r, "accountID")
	if err != nil {
		return nil, httpx.NotFound("account not found")
	}
	return h.accessibleAccountByID(r.Context(), CurrentUser(r).ID, id)
}

// accessibleAccountByID is AccessibleAccount without the *http.Request, for
// callers (the MCP tools) that get the account id from somewhere other
// than a {accountID} path value.
func (h *AccountHandler) accessibleAccountByID(ctx context.Context, viewerID, id uuid.UUID) (*entities.Account, error) {
	account, err := h.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, httpx.NotFound("account not found")
	}
	ok, err := h.canAccessOwner(ctx, viewerID, account.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.NotFound("account not found")
	}
	return account, nil
}

// RequireOwnAccount behaves like AccessibleAccount but rejects shared
// (collaborator) access — for the handful of account-scoped actions
// collaborators aren't granted: importing statements, managing balance
// checkpoints, editing a recurring item.
func (h *AccountHandler) RequireOwnAccount(r *http.Request) (*entities.Account, error) {
	account, err := h.AccessibleAccount(r)
	if err != nil {
		return nil, err
	}
	if account.UserID != CurrentUser(r).ID {
		return nil, httpx.Forbidden("only the account owner can do this")
	}
	return account, nil
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.listAccessible(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// listAccessible returns viewerID's own accounts plus every account shared
// with them.
func (h *AccountHandler) listAccessible(ctx context.Context, viewerID uuid.UUID) ([]accountResponse, error) {
	ownerIDs, err := h.ResolveOwnerIDs(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	var accounts []entities.Account
	for _, ownerID := range ownerIDs {
		forOwner, err := h.accounts.ListForUser(ctx, ownerID)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, forOwner...)
	}
	out := make([]accountResponse, len(accounts))
	for i, a := range accounts {
		out[i] = h.toAccountResponse(ctx, a, viewerID)
	}
	return out, nil
}

type createAccountRequest struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createAccountRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	currency := money.DefaultCurrency
	if body.Currency != "" {
		var err error
		if currency, err = money.ParseCurrency(body.Currency); err != nil {
			httpx.WriteError(w, httpx.BadRequest("unsupported currency"))
			return
		}
	}
	account := &entities.Account{ID: uuid.New(), UserID: CurrentUser(r).ID, Name: body.Name, Currency: currency}
	created, err := h.accounts.Add(r.Context(), account)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.toAccountResponse(r.Context(), *created, CurrentUser(r).ID))
}

func (h *AccountHandler) Get(w http.ResponseWriter, r *http.Request) {
	account, err := h.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.toAccountResponse(r.Context(), *account, CurrentUser(r).ID))
}

// Delete permanently removes the account and everything in it (operations,
// planned items, checkpoints, import history - see migration 000014's
// cascade). Owner-only, same as import/checkpoints/recurring-item edits -
// a collaborator can view and categorize but not do something this
// destructive and irreversible.
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	account, err := h.RequireOwnAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.accounts.Delete(r.Context(), account.ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
