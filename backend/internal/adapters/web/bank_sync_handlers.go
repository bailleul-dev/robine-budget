package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

type BankSyncHandler struct {
	connect *services.ConnectBankUseCase
	sync    *services.SyncBankAccountsUseCase
	status  *services.BankConnectionStatusUseCase

	// Bank sync stores a person's online-banking credentials and scrapes
	// their bank: acceptable for the operator's own account, not for the
	// public. Until an approved aggregator replaces it, it is closed to
	// everyone except an explicit allow-list of verified addresses. The zero
	// value denies everybody.
	users     ports.UserRepository
	allowed   map[string]bool
	allowEver bool
}

func NewBankSyncHandler(connect *services.ConnectBankUseCase, sync *services.SyncBankAccountsUseCase, status *services.BankConnectionStatusUseCase) *BankSyncHandler {
	return &BankSyncHandler{connect: connect, sync: sync, status: status}
}

// WithAllowList opens bank sync to the accounts whose *verified* email is in
// emails (case-insensitive). An empty list keeps it closed to everyone.
func (h *BankSyncHandler) WithAllowList(users ports.UserRepository, emails []string) *BankSyncHandler {
	h.users = users
	h.allowed = map[string]bool{}
	for _, e := range emails {
		h.allowed[strings.ToLower(strings.TrimSpace(e))] = true
	}
	return h
}

// AllowAll opens bank sync to every account, verified or not — for tests only.
func (h *BankSyncHandler) AllowAll() *BankSyncHandler { h.allowEver = true; return h }

// available reports whether bank sync is open to this user.
func (h *BankSyncHandler) available(user *entities.User) bool {
	if h.allowEver {
		return user != nil
	}
	if user == nil || !user.EmailVerified() || user.Email == nil {
		return false
	}
	return h.allowed[strings.ToLower(*user.Email)]
}

// availableForID is available for callers that only have a user id (the MCP
// tools).
func (h *BankSyncHandler) availableForID(ctx context.Context, userID uuid.UUID) bool {
	if h.allowEver {
		return true
	}
	if h.users == nil {
		return false
	}
	user, err := h.users.GetByID(ctx, userID)
	return err == nil && h.available(user)
}

var errBankSyncUnavailable = httpx.Forbidden("bank sync is not available for your account")

type bankConnectionStatusResponse struct {
	Connected bool `json:"connected"`
	// Available is false when bank sync is closed to this account; clients
	// should then hide the feature instead of offering a form that will 403.
	Available bool   `json:"available"`
	Bank      string `json:"bank,omitempty"`
}

// Status reports whether this user already has a bank connected — the
// client uses it to decide between showing the connect form or a plain
// sync action.
func (h *BankSyncHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.available(CurrentUser(r)) {
		httpx.WriteJSON(w, http.StatusOK, bankConnectionStatusResponse{Connected: false, Available: false})
		return
	}
	status, err := h.status.Status(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	resp := bankConnectionStatusResponse{Connected: status.Connected, Available: true}
	if status.Connected {
		resp.Bank = string(status.Bank)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// bankSyncResultResponse is deliberately a summary, not the fetched
// accounts/balances - those already have a home (GET /api/accounts,
// GET /api/accounts/{id}/... checkpoints); this endpoint's job is to
// trigger the reconciliation, not to be another way to read the data.
type bankSyncResultResponse struct {
	SyncedAccounts  int `json:"synced_accounts"`
	CreatedAccounts int `json:"created_accounts"`
	OperationsAdded int `json:"operations_added"`
}

func toBankSyncResultResponse(accounts []services.SyncedAccount) bankSyncResultResponse {
	resp := bankSyncResultResponse{SyncedAccounts: len(accounts)}
	for _, a := range accounts {
		if a.Created {
			resp.CreatedAccounts++
		}
		resp.OperationsAdded += a.OperationsAdded
	}
	return resp
}

// writeBankSyncResult surfaces a *services.BankSyncError (bad credentials,
// app-validation refused/expired, already connected) as a 400 with its
// message; anything else is an infrastructure failure, reported as an
// opaque 500 like every other handler in this codebase.
func writeBankSyncResult(w http.ResponseWriter, accounts []services.SyncedAccount, err error) {
	if err != nil {
		var syncErr *services.BankSyncError
		if errors.As(err, &syncErr) {
			httpx.WriteError(w, httpx.BadRequest(syncErr.Error()))
			return
		}
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toBankSyncResultResponse(accounts))
}

type connectBankRequest struct {
	// Bank picks which aggregator backend to connect - only "cm" (Credit
	// Mutuel) is supported for now (services.ConnectBankUseCase rejects
	// anything else), but the field is required from the start so adding a
	// second bank later doesn't need an API change.
	Bank     string `json:"bank"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Connect links this user's bank account for the first time: stores the
// credentials (encrypted) and runs the first sync - which is also where the
// initial app-validation (2FA) happens.
func (h *BankSyncHandler) Connect(w http.ResponseWriter, r *http.Request) {
	if !h.available(CurrentUser(r)) {
		httpx.WriteError(w, errBankSyncUnavailable)
		return
	}
	var body connectBankRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if body.Bank == "" || body.Login == "" || body.Password == "" {
		httpx.WriteError(w, httpx.BadRequest("bank, login and password are required"))
		return
	}
	accounts, err := h.connect.Connect(r.Context(), CurrentUser(r).ID, entities.BankSyncBank(body.Bank), body.Login, body.Password)
	writeBankSyncResult(w, accounts, err)
}

// Sync re-fetches the already-connected bank's accounts.
func (h *BankSyncHandler) Sync(w http.ResponseWriter, r *http.Request) {
	if !h.available(CurrentUser(r)) {
		httpx.WriteError(w, errBankSyncUnavailable)
		return
	}
	accounts, err := h.sync.Sync(r.Context(), CurrentUser(r).ID)
	writeBankSyncResult(w, accounts, err)
}
