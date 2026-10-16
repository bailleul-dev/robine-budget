package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newTestPrevisionsHandler() (*PrevisionsHandler, *fakeOperationRepo, *fakeBalanceCheckpointRepo, *fakeAccountRepo) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeBalanceCheckpointRepo()
	previsions := services.NewPrevisionsUseCase(operations, occurrences, checkpoints)
	accountHandler, accounts, _, _ := newTestAccountHandler()
	return NewPrevisionsHandler(previsions, accountHandler), operations, checkpoints, accounts
}

func TestPrevisionsHandler_Get_Success(t *testing.T) {
	h, operations, checkpoints, accounts := newTestPrevisionsHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	checkpoint := &entities.BalanceCheckpoint{ID: uuid.New(), AccountID: account.ID, AsOfDate: civil.New(2025, 12, 31), Balance: mustAmount(t, "1000.00")}
	checkpoints.byID[checkpoint.ID] = checkpoint
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.New(2026, 1, 10), Label: "Groceries", Amount: mustAmount(t, "-50.00")}
	operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/previsions?date_from=2026-01-01&date_to=2026-01-31", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp previsionsResponse
	decodeJSONBody(t, rec, &resp)
	if resp.OpeningBalance != "1000.00" {
		t.Errorf("OpeningBalance = %q, want 1000.00", resp.OpeningBalance)
	}
	if resp.ClosingBalance != "950.00" {
		t.Errorf("ClosingBalance = %q, want 950.00", resp.ClosingBalance)
	}
}

func TestPrevisionsHandler_Get_MissingDateRange(t *testing.T) {
	h, _, _, accounts := newTestPrevisionsHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/previsions", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (date_from/date_to required), body=%s", rec.Code, rec.Body)
	}
}

func TestPrevisionsHandler_Get_InvalidDate(t *testing.T) {
	h, _, _, accounts := newTestPrevisionsHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/previsions?date_from=not-a-date&date_to=2026-01-31", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestPrevisionsHandler_Get_AccountNotAccessible(t *testing.T) {
	h, _, _, _ := newTestPrevisionsHandler()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString()+"/previsions?date_from=2026-01-01&date_to=2026-01-31", nil, map[string]string{"accountID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}
