package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newTestBalanceCheckpointHandler() (*BalanceCheckpointHandler, *fakeBalanceCheckpointRepo, *fakeAccountRepo) {
	checkpoints := newFakeBalanceCheckpointRepo()
	accountHandler, accounts, _, _ := newTestAccountHandler()
	operations := newFakeOperationRepo()
	computeInitial := services.NewComputeInitialBalanceUseCase(operations, checkpoints)
	return NewBalanceCheckpointHandler(checkpoints, accountHandler, computeInitial), checkpoints, accounts
}

func TestBalanceCheckpointHandler_Create_OwnerSucceeds(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	body := createCheckpointRequest{AsOfDate: "2026-01-15", Balance: "1234.56"}
	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/balance-checkpoints", body, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body)
	}
	var resp balanceCheckpointResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Balance != "1234.56" {
		t.Errorf("Balance = %q, want 1234.56", resp.Balance)
	}
	if len(checkpoints.byID) != 1 {
		t.Fatalf("got %d checkpoints stored, want 1", len(checkpoints.byID))
	}
}

func TestBalanceCheckpointHandler_Create_ForbiddenForCollaborator(t *testing.T) {
	h, _, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	body := createCheckpointRequest{AsOfDate: "2026-01-15", Balance: "100.00"}
	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/balance-checkpoints", body, map[string]string{"accountID": account.ID.String()}), collaborator)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 404 {
		// AccessibleAccount 404s (not 403) since the collaborator has no
		// grant on this account at all in this test - see RequireOwnAccount.
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestBalanceCheckpointHandler_Create_InvalidBalance(t *testing.T) {
	h, _, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	body := createCheckpointRequest{AsOfDate: "2026-01-15", Balance: "not-a-number"}
	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/balance-checkpoints", body, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestBalanceCheckpointHandler_List(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	cp := &entities.BalanceCheckpoint{ID: uuid.New(), AccountID: account.ID, AsOfDate: civil.Today(), Balance: mustAmount(t, "500.00")}
	checkpoints.byID[cp.ID] = cp

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/balance-checkpoints", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []balanceCheckpointResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d checkpoints, want 1", len(resp))
	}
}

func TestBalanceCheckpointHandler_ComputeInitial_Success(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	body := createCheckpointRequest{AsOfDate: "2026-01-15", Balance: "1000.00"}
	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/balance-checkpoints/initial", body, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.ComputeInitial(rec, req)

	if rec.Code != 201 {
		t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body)
	}
	if len(checkpoints.byID) != 1 {
		t.Fatalf("got %d checkpoints stored, want 1", len(checkpoints.byID))
	}
}

func TestBalanceCheckpointHandler_ComputeInitial_AlreadySet(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	existing := &entities.BalanceCheckpoint{ID: uuid.New(), AccountID: account.ID, AsOfDate: civil.Today(), Balance: mustAmount(t, "500.00")}
	checkpoints.byID[existing.ID] = existing

	body := createCheckpointRequest{AsOfDate: "2026-01-15", Balance: "1000.00"}
	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/balance-checkpoints/initial", body, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.ComputeInitial(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (an account may only be bootstrapped once), body=%s", rec.Code, rec.Body)
	}
}

func TestBalanceCheckpointHandler_Delete_NotOwner(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	stranger := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	cp := &entities.BalanceCheckpoint{ID: uuid.New(), AccountID: account.ID, AsOfDate: civil.Today(), Balance: mustAmount(t, "500.00")}
	checkpoints.byID[cp.ID] = cp

	req := asUser(jsonRequest(t, "DELETE", "/api/balance-checkpoints/"+cp.ID.String(), nil, map[string]string{"checkpointID": cp.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestBalanceCheckpointHandler_Delete_Success(t *testing.T) {
	h, checkpoints, accounts := newTestBalanceCheckpointHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	cp := &entities.BalanceCheckpoint{ID: uuid.New(), AccountID: account.ID, AsOfDate: civil.Today(), Balance: mustAmount(t, "500.00")}
	checkpoints.byID[cp.ID] = cp

	req := asUser(jsonRequest(t, "DELETE", "/api/balance-checkpoints/"+cp.ID.String(), nil, map[string]string{"checkpointID": cp.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body)
	}
}
