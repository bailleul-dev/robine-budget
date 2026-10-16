package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

type reconciliationFixture struct {
	handler        *ReconciliationHandler
	operations     *fakeOperationRepo
	occurrences    *fakeOccurrenceRepo
	plannedItems   *fakePlannedItemRepo
	accountHandler *AccountHandler
	accounts       *fakeAccountRepo
}

func newReconciliationFixture() *reconciliationFixture {
	accountHandler, accounts, _, _ := newTestAccountHandler()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	reconcile := services.NewReconcileUseCase(operations, occurrences, plannedItems)
	handler := NewReconciliationHandler(operations, occurrences, plannedItems, accountHandler, reconcile)
	return &reconciliationFixture{
		handler: handler, operations: operations, occurrences: occurrences,
		plannedItems: plannedItems, accountHandler: accountHandler, accounts: accounts,
	}
}

func TestReconciliationHandler_ListUnreconciledOperations(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Unmatched", Amount: mustAmount(t, "-15.00")}
	f.operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/reconciliation/unreconciled-operations", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListUnreconciledOperations(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []operationSummaryResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d operations, want 1", len(resp))
	}
}

func TestReconciliationHandler_Reconcile_Success(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	category := uuid.New()
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), CategoryID: &category, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occurrence := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00"), Status: entities.OccurrenceForecast}
	f.occurrences.byID[occurrence.ID] = occurrence
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Rent payment", Amount: mustAmount(t, "-800.00")}
	f.operations.byID[op.ID] = op

	body := reconcileRequest{OccurrenceID: occurrence.ID.String(), OperationID: op.ID.String()}
	req := asUser(jsonRequest(t, "POST", "/api/reconciliation", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Reconcile(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp occurrenceResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Status != string(entities.OccurrenceReconciled) {
		t.Errorf("Status = %q, want reconciled", resp.Status)
	}
	if resp.ReconciledOperationID == nil || *resp.ReconciledOperationID != op.ID.String() {
		t.Errorf("ReconciledOperationID = %v, want %s", resp.ReconciledOperationID, op.ID)
	}
	if op.CategoryID == nil || *op.CategoryID != category {
		t.Error("reconciling should carry the planned item's category onto the operation")
	}
}

func TestReconciliationHandler_Reconcile_OccurrenceNotAccessible(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	stranger := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occurrence := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00")}
	f.occurrences.byID[occurrence.ID] = occurrence

	body := reconcileRequest{OccurrenceID: occurrence.ID.String(), OperationID: uuid.NewString()}
	req := asUser(jsonRequest(t, "POST", "/api/reconciliation", body, nil), stranger)
	rec := httptest.NewRecorder()
	f.handler.Reconcile(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestReconciliationHandler_Unreconcile_Success(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	opID := uuid.New()
	occurrence := &entities.PlannedOccurrence{
		ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00"),
		Status: entities.OccurrenceReconciled, ReconciledOperationID: &opID,
	}
	f.occurrences.byID[occurrence.ID] = occurrence

	req := asUser(jsonRequest(t, "DELETE", "/api/reconciliation/"+occurrence.ID.String(), nil, map[string]string{"occurrenceID": occurrence.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Unreconcile(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp occurrenceResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Status != string(entities.OccurrenceForecast) {
		t.Errorf("Status = %q, want forecast", resp.Status)
	}
	if resp.ReconciledOperationID != nil {
		t.Error("ReconciledOperationID should be cleared after unreconciling")
	}
}

func TestReconciliationHandler_Suggestions_MatchesByAmountAndDate(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	f.occurrences.itemAccount[item.ID] = account.ID
	occurrence := &entities.PlannedOccurrence{
		ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.New(2026, 1, 5),
		ExpectedAmount: mustAmount(t, "-800.00"), Status: entities.OccurrenceForecast,
	}
	f.occurrences.byID[occurrence.ID] = occurrence
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.New(2026, 1, 5), Label: "Rent payment", Amount: mustAmount(t, "-800.00")}
	f.operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/reconciliation/suggestions", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Suggestions(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []suggestionResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(resp))
	}
	if resp[0].Operation.ID != op.ID.String() {
		t.Errorf("suggested operation ID = %q, want %s", resp[0].Operation.ID, op.ID)
	}
}

func TestReconciliationHandler_Suggestions_NotAccessible(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString()+"/reconciliation/suggestions", nil, map[string]string{"accountID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Suggestions(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestReconciliationHandler_Unreconcile_NotFound(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "DELETE", "/api/reconciliation/"+uuid.NewString(), nil, map[string]string{"occurrenceID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Unreconcile(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

// Security regression: operation_id comes from the request body, so the
// handler must check the operation itself, not just the occurrence. Before,
// reconciling your own occurrence with someone else's operation id wrote your
// category onto their operation and returned their amount.
func TestReconciliationHandler_Reconcile_OperationOfAnotherUserIsRefused(t *testing.T) {
	f := newReconciliationFixture()
	attacker := newTestUser()
	victim := newTestUser()

	attackerAccount := &entities.Account{ID: uuid.New(), UserID: attacker.ID, Name: "Mine", Currency: "EUR"}
	victimAccount := &entities.Account{ID: uuid.New(), UserID: victim.ID, Name: "Theirs", Currency: "EUR"}
	f.accounts.byID[attackerAccount.ID] = attackerAccount
	f.accounts.byID[victimAccount.ID] = victimAccount

	category := uuid.New()
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: attackerAccount.ID, Label: strPtr("Rent"), CategoryID: &category, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occurrence := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00"), Status: entities.OccurrenceForecast}
	f.occurrences.byID[occurrence.ID] = occurrence
	victimOp := &entities.Operation{ID: uuid.New(), AccountID: victimAccount.ID, Date: civil.Today(), Label: "Secret salary", Amount: mustAmount(t, "4321.00")}
	f.operations.byID[victimOp.ID] = victimOp

	body := reconcileRequest{OccurrenceID: occurrence.ID.String(), OperationID: victimOp.ID.String()}
	rec := httptest.NewRecorder()
	f.handler.Reconcile(rec, asUser(jsonRequest(t, "POST", "/api/reconciliation", body, nil), attacker))

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "4321") {
		t.Error("the victim's amount leaked in the response")
	}
	if victimOp.CategoryID != nil {
		t.Error("the attacker's category was written onto the victim's operation")
	}
	if occurrence.Status == entities.OccurrenceReconciled {
		t.Error("the attacker's occurrence was reconciled against a foreign operation")
	}
}

// An operation on a *different account of the same user* is not the
// occurrence's account either: reconciliation is per account.
func TestReconciliationHandler_Reconcile_OperationOfAnotherAccountIsRefused(t *testing.T) {
	f := newReconciliationFixture()
	owner := newTestUser()
	a := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "A", Currency: "EUR"}
	b := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "B", Currency: "EUR"}
	f.accounts.byID[a.ID] = a
	f.accounts.byID[b.ID] = b
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: a.ID, Label: strPtr("Rent"), MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occurrence := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00")}
	f.occurrences.byID[occurrence.ID] = occurrence
	op := &entities.Operation{ID: uuid.New(), AccountID: b.ID, Date: civil.Today(), Label: "x", Amount: mustAmount(t, "-800.00")}
	f.operations.byID[op.ID] = op

	rec := httptest.NewRecorder()
	f.handler.Reconcile(rec, asUser(jsonRequest(t, "POST", "/api/reconciliation", reconcileRequest{OccurrenceID: occurrence.ID.String(), OperationID: op.ID.String()}, nil), owner))
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}
