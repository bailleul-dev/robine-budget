package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func newTestAccountHandler() (*AccountHandler, *fakeAccountRepo, *fakeUserRepo, *fakeCollaboratorRepo) {
	accounts := newFakeAccountRepo()
	users := newFakeUserRepo()
	collaborators := newFakeCollaboratorRepo()
	return NewAccountHandler(accounts, users, collaborators), accounts, users, collaborators
}

func TestAccountHandler_Create(t *testing.T) {
	h, accounts, _, _ := newTestAccountHandler()
	user := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/accounts", createAccountRequest{Name: "Checking"}, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp accountResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Name != "Checking" {
		t.Errorf("Name = %q, want Checking", resp.Name)
	}
	if resp.Currency != "EUR" {
		t.Errorf("Currency = %q, want EUR (default)", resp.Currency)
	}
	if len(accounts.byID) != 1 {
		t.Fatalf("got %d accounts stored, want 1", len(accounts.byID))
	}
}

func TestAccountHandler_Create_NormalizesCurrency(t *testing.T) {
	h, _, _, _ := newTestAccountHandler()
	req := asUser(jsonRequest(t, "POST", "/api/accounts", createAccountRequest{Name: "US", Currency: "usd"}, nil), newTestUser())
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp accountResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", resp.Currency)
	}
}

func TestAccountHandler_Create_RejectsUnsupportedCurrency(t *testing.T) {
	h, accounts, _, _ := newTestAccountHandler()
	req := asUser(jsonRequest(t, "POST", "/api/accounts", createAccountRequest{Name: "Typo", Currency: "EUU"}, nil), newTestUser())
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
	if len(accounts.byID) != 0 {
		t.Fatalf("got %d accounts stored, want 0", len(accounts.byID))
	}
}

func TestAccountHandler_Get_OwnAccount(t *testing.T) {
	h, accounts, _, _ := newTestAccountHandler()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Savings", Currency: "EUR"}
	accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp accountResponse
	decodeJSONBody(t, rec, &resp)
	if resp.ID != account.ID.String() {
		t.Errorf("ID = %q, want %s", resp.ID, account.ID)
	}
	if resp.OwnerDisplayName != nil {
		t.Errorf("OwnerDisplayName = %v, want nil for the viewer's own account", resp.OwnerDisplayName)
	}
}

func TestAccountHandler_Get_NotFoundForStranger(t *testing.T) {
	h, accounts, _, _ := newTestAccountHandler()
	owner := newTestUser()
	stranger := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Savings", Currency: "EUR"}
	accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (must not leak existence to an unrelated user), body=%s", rec.Code, rec.Body)
	}
}

func TestAccountHandler_Get_VisibleToCollaborator(t *testing.T) {
	h, accounts, _, collaborators := newTestAccountHandler()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Shared", Currency: "EUR"}
	accounts.byID[account.ID] = account
	grant := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, CollaboratorUserID: &collaborator.ID}
	acceptedAt := account.CreatedAt
	grant.AcceptedAt = &acceptedAt
	collaborators.byID[grant.ID] = grant

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), collaborator)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (an active collaborator grant should allow access), body=%s", rec.Code, rec.Body)
	}
}

func TestAccountHandler_Get_NotVisibleAfterRevoke(t *testing.T) {
	h, accounts, _, collaborators := newTestAccountHandler()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Shared", Currency: "EUR"}
	accounts.byID[account.ID] = account
	acceptedAt := account.CreatedAt
	revokedAt := account.CreatedAt
	grant := &entities.Collaborator{
		ID: uuid.New(), OwnerUserID: owner.ID, CollaboratorUserID: &collaborator.ID,
		AcceptedAt: &acceptedAt, RevokedAt: &revokedAt,
	}
	collaborators.byID[grant.ID] = grant

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), collaborator)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (a revoked grant must no longer allow access), body=%s", rec.Code, rec.Body)
	}
}

func TestAccountHandler_Get_UnknownID(t *testing.T) {
	h, _, _, _ := newTestAccountHandler()
	user := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString(), nil, map[string]string{"accountID": uuid.NewString()}), user)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAccountHandler_List_IncludesOwnAndSharedAccounts(t *testing.T) {
	h, accounts, _, collaborators := newTestAccountHandler()
	viewer := newTestUser()
	otherOwner := newTestUser()

	own := &entities.Account{ID: uuid.New(), UserID: viewer.ID, Name: "Mine", Currency: "EUR"}
	shared := &entities.Account{ID: uuid.New(), UserID: otherOwner.ID, Name: "Shared", Currency: "EUR"}
	accounts.byID[own.ID] = own
	accounts.byID[shared.ID] = shared

	acceptedAt := own.CreatedAt
	grant := &entities.Collaborator{ID: uuid.New(), OwnerUserID: otherOwner.ID, CollaboratorUserID: &viewer.ID, AcceptedAt: &acceptedAt}
	collaborators.byID[grant.ID] = grant

	req := asUser(jsonRequest(t, "GET", "/api/accounts", nil, nil), viewer)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []accountResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 2 {
		t.Fatalf("got %d accounts, want 2 (own + shared)", len(resp))
	}
}

func TestAccountHandler_Delete_ForbiddenForCollaborator(t *testing.T) {
	h, accounts, _, collaborators := newTestAccountHandler()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Shared", Currency: "EUR"}
	accounts.byID[account.ID] = account
	acceptedAt := account.CreatedAt
	grant := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, CollaboratorUserID: &collaborator.ID, AcceptedAt: &acceptedAt}
	collaborators.byID[grant.ID] = grant

	req := asUser(jsonRequest(t, "DELETE", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), collaborator)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (only the owner may delete), body=%s", rec.Code, rec.Body)
	}
	if _, stillThere := accounts.byID[account.ID]; !stillThere {
		t.Error("account should not have been deleted")
	}
}

func TestAccountHandler_Delete_OwnerSucceeds(t *testing.T) {
	h, accounts, _, _ := newTestAccountHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Mine", Currency: "EUR"}
	accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "DELETE", "/api/accounts/"+account.ID.String(), nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body)
	}
	if _, stillThere := accounts.byID[account.ID]; stillThere {
		t.Error("account should have been deleted")
	}
}
