package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

type plannedItemFixture struct {
	handler        *PlannedItemHandler
	plannedItems   *fakePlannedItemRepo
	occurrences    *fakeOccurrenceRepo
	categories     *fakeCategoryRepo
	operations     *fakeOperationRepo
	accountHandler *AccountHandler
	accounts       *fakeAccountRepo
}

func newPlannedItemFixture() *plannedItemFixture {
	plannedItems := newFakePlannedItemRepo()
	occurrences := newFakeOccurrenceRepo()
	categories := newFakeCategoryRepo()
	operations := newFakeOperationRepo()
	accountHandler, accounts, _, _ := newTestAccountHandler()
	generateOccurrences := services.NewGenerateOccurrencesUseCase(plannedItems, occurrences)
	autoMatch := services.NewReconcileUseCase(operations, occurrences, plannedItems)
	detectRecurring := services.NewRecurringDetectionUseCase(operations, plannedItems)
	handler := NewPlannedItemHandler(plannedItems, occurrences, categories, generateOccurrences, autoMatch, detectRecurring, accountHandler)
	return &plannedItemFixture{
		handler: handler, plannedItems: plannedItems, occurrences: occurrences, categories: categories,
		operations: operations, accountHandler: accountHandler, accounts: accounts,
	}
}

func validPlannedItemBody(accountID uuid.UUID) plannedItemRequest {
	dayOfMonth := 5
	return plannedItemRequest{
		AccountID: accountID.String(), Kind: string(entities.PlannedItemKindCharge), Label: strPtr("Rent"),
		Amount: "800.00", StartDate: "2026-01-05", RecurrenceType: string(entities.RecurrencePerpetual),
		DayOfMonth: &dayOfMonth, Active: true,
	}
}

func TestPlannedItemHandler_Create_Success(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "POST", "/api/planned-items", validPlannedItemBody(account.ID), nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp plannedItemResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Label == nil || *resp.Label != "Rent" {
		t.Errorf("Label = %v, want Rent", resp.Label)
	}
	if len(f.plannedItems.byID) != 1 {
		t.Fatalf("got %d planned items stored, want 1", len(f.plannedItems.byID))
	}
}

func TestPlannedItemHandler_Create_ByCollaboratorAllowed(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	acceptedAt := account.CreatedAt
	grant := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, CollaboratorUserID: &collaborator.ID, AcceptedAt: &acceptedAt}
	collaborators := newFakeCollaboratorRepo()
	collaborators.byID[grant.ID] = grant
	// Rebuild the account handler wired to a collaborator repo containing
	// the grant above - NewAccountHandler's collaborators field is private,
	// so this fixture is reconstructed here rather than mutated in place.
	accountHandler2 := NewAccountHandler(f.accounts, newFakeUserRepo(), collaborators)
	f.handler = NewPlannedItemHandler(f.plannedItems, f.occurrences, f.categories,
		services.NewGenerateOccurrencesUseCase(f.plannedItems, f.occurrences),
		services.NewReconcileUseCase(f.operations, f.occurrences, f.plannedItems),
		services.NewRecurringDetectionUseCase(f.operations, f.plannedItems),
		accountHandler2)

	req := asUser(jsonRequest(t, "POST", "/api/planned-items", validPlannedItemBody(account.ID), nil), collaborator)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (a collaborator may create a recurring item), body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Create_InvalidRecurrenceValidation(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	body := plannedItemRequest{
		AccountID: account.ID.String(), Kind: string(entities.PlannedItemKindCharge), Label: strPtr("Rent"),
		Amount: "800.00", StartDate: "2026-01-05", RecurrenceType: string(entities.RecurrenceFixedCount),
		// OccurrenceCount deliberately omitted - required for fixed_count.
	}
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (occurrence_count required), body=%s", rec.Code, rec.Body)
	}
}

// An unknown enum value is the caller's mistake (400), not something to
// pass through to Postgres and fail there as a 500.
func TestPlannedItemHandler_Create_RejectsUnknownEnumValues(t *testing.T) {
	cases := map[string]func(*plannedItemRequest){
		"kind":            func(b *plannedItemRequest) { b.Kind = "expense" },
		"match_mode":      func(b *plannedItemRequest) { b.MatchMode = "amount" },
		"recurrence_type": func(b *plannedItemRequest) { b.RecurrenceType = "yearly" },
	}
	for field, mutate := range cases {
		t.Run(field, func(t *testing.T) {
			f := newPlannedItemFixture()
			owner := newTestUser()
			account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
			f.accounts.byID[account.ID] = account

			body := validPlannedItemBody(account.ID)
			mutate(&body)
			req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
			rec := httptest.NewRecorder()
			f.handler.Create(rec, req)

			if rec.Code != 400 {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
			}
			if len(f.plannedItems.byID) != 0 {
				t.Fatalf("got %d planned items stored, want 0", len(f.plannedItems.byID))
			}
		})
	}
}

func TestPlannedItemHandler_Create_EveryXWeeksRequiresIntervalWeeks(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	body := plannedItemRequest{
		AccountID: account.ID.String(), Kind: string(entities.PlannedItemKindCharge), Label: strPtr("Gym"),
		Amount: "30.00", StartDate: "2026-01-05", RecurrenceType: string(entities.RecurrenceEveryXWeeks),
	}
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (interval_weeks required), body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Create_InvalidAmount(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	body := validPlannedItemBody(account.ID)
	body.Amount = "not-a-number"
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Create_InvalidStartDate(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	body := validPlannedItemBody(account.ID)
	body.StartDate = "not-a-date"
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Create_UnknownAccount(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/planned-items", validPlannedItemBody(uuid.New()), nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Create_WithCategory(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	category := &entities.Category{ID: uuid.New(), Name: "Housing"}
	f.categories.byID[category.ID] = category

	body := validPlannedItemBody(account.ID)
	categoryID := category.ID.String()
	body.CategoryID = &categoryID
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp plannedItemResponse
	decodeJSONBody(t, rec, &resp)
	if resp.CategoryID == nil || *resp.CategoryID != category.ID.String() {
		t.Errorf("CategoryID = %v, want %s", resp.CategoryID, category.ID)
	}
}

func TestPlannedItemHandler_Create_InvalidCategory(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	body := validPlannedItemBody(account.ID)
	unknownCategory := uuid.NewString()
	body.CategoryID = &unknownCategory
	req := asUser(jsonRequest(t, "POST", "/api/planned-items", body, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Update_OwnerSucceeds(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Old"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item

	body := validPlannedItemBody(account.ID)
	body.Label = strPtr("New Label")
	req := asUser(jsonRequest(t, "PUT", "/api/planned-items/"+item.ID.String(), body, map[string]string{"itemID": item.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Update(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp plannedItemResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Label == nil || *resp.Label != "New Label" {
		t.Errorf("Label = %v, want New Label", resp.Label)
	}
}

func TestPlannedItemHandler_Update_ForbiddenForCollaborator(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	acceptedAt := account.CreatedAt
	collaborators := newFakeCollaboratorRepo()
	grant := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, CollaboratorUserID: &collaborator.ID, AcceptedAt: &acceptedAt}
	collaborators.byID[grant.ID] = grant
	accountHandler2 := NewAccountHandler(f.accounts, newFakeUserRepo(), collaborators)
	f.handler = NewPlannedItemHandler(f.plannedItems, f.occurrences, f.categories,
		services.NewGenerateOccurrencesUseCase(f.plannedItems, f.occurrences),
		services.NewReconcileUseCase(f.operations, f.occurrences, f.plannedItems),
		services.NewRecurringDetectionUseCase(f.operations, f.plannedItems),
		accountHandler2)

	body := validPlannedItemBody(account.ID)
	req := asUser(jsonRequest(t, "PUT", "/api/planned-items/"+item.ID.String(), body, map[string]string{"itemID": item.ID.String()}), collaborator)
	rec := httptest.NewRecorder()
	f.handler.Update(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403 (only the owner may edit a recurring item), body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_Update_NotFound(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "PUT", "/api/planned-items/"+uuid.NewString(), validPlannedItemBody(uuid.New()), map[string]string{"itemID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.Update(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_ListAll(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item

	req := asUser(jsonRequest(t, "GET", "/api/planned-items", nil, nil), owner)
	rec := httptest.NewRecorder()
	f.handler.ListAll(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_ListForAccount(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/planned-items", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListForAccount(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []plannedItemResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d items, want 1", len(resp))
	}
}

func TestPlannedItemHandler_ListForAccount_NotAccessible(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString()+"/planned-items", nil, map[string]string{"accountID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListForAccount(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_GenerateOccurrences_Success(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	dayOfMonth := 5
	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge,
		Amount: mustAmount(t, "800.00"), StartDate: civil.New(2026, 1, 5), RecurrenceType: entities.RecurrencePerpetual,
		DayOfMonth: &dayOfMonth, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,}
	f.plannedItems.byID[item.ID] = item

	req := asUser(jsonRequest(t, "POST", "/api/planned-items/"+item.ID.String()+"/generate-occurrences?months_ahead=3", nil, map[string]string{"itemID": item.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.GenerateOccurrences(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []occurrenceResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) == 0 {
		t.Error("expected at least one generated occurrence")
	}
}

func TestPlannedItemHandler_GenerateOccurrences_InvalidMonthsAhead(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item

	req := asUser(jsonRequest(t, "POST", "/api/planned-items/"+item.ID.String()+"/generate-occurrences?months_ahead=0", nil, map[string]string{"itemID": item.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.GenerateOccurrences(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (months_ahead out of range), body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_GenerateOccurrences_NotAccessible(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/planned-items/"+uuid.NewString()+"/generate-occurrences", nil, map[string]string{"itemID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.GenerateOccurrences(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_ListOccurrences(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00")}
	f.occurrences.byID[occ.ID] = occ

	req := asUser(jsonRequest(t, "GET", "/api/planned-items/"+item.ID.String()+"/occurrences", nil, map[string]string{"itemID": item.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListOccurrences(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []occurrenceResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(resp))
	}
}

func TestPlannedItemHandler_RecurringSuggestions(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/recurring-suggestions", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.RecurringSuggestions(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []recurringSuggestionResponse
	decodeJSONBody(t, rec, &resp)
	if resp == nil {
		t.Error("expected a (possibly empty) JSON array, got null")
	}
}

func TestPlannedItemHandler_RecurringSuggestions_DetectsPattern(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	// Same label, same day of month, three consecutive months - the
	// minimum shape RecurringDetectionUseCase.Detect requires to suggest a
	// monthly recurring item (see minRecurringOccurrences).
	for month := 1; month <= 3; month++ {
		op := &entities.Operation{
			ID: uuid.New(), AccountID: account.ID, Date: civil.New(2026, time.Month(month), 5),
			Label: "Netflix", Amount: mustAmount(t, "-15.99"),
		}
		f.operations.byID[op.ID] = op
	}

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/recurring-suggestions", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.RecurringSuggestions(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []recurringSuggestionResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(resp))
	}
	if resp[0].Label != "Netflix" {
		t.Errorf("Label = %q, want Netflix", resp[0].Label)
	}
	if resp[0].OccurrenceCount != 3 {
		t.Errorf("OccurrenceCount = %d, want 3", resp[0].OccurrenceCount)
	}
}

func TestPlannedItemHandler_RecurringSuggestions_NotAccessible(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString()+"/recurring-suggestions", nil, map[string]string{"accountID": uuid.NewString()}), owner)
	rec := httptest.NewRecorder()
	f.handler.RecurringSuggestions(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestPlannedItemHandler_ListOccurrencesForAccount(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	item := &entities.PlannedItem{ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	f.plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustAmount(t, "-800.00")}
	f.occurrences.byID[occ.ID] = occ
	f.occurrences.itemAccount[item.ID] = account.ID

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/planned-occurrences", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListOccurrencesForAccount(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []occurrenceResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 {
		t.Fatalf("got %d occurrences, want 1", len(resp))
	}
}

func TestPlannedItemHandler_ListOccurrencesForAccount_InvalidDateRange(t *testing.T) {
	f := newPlannedItemFixture()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/planned-occurrences?date_from=not-a-date", nil, map[string]string{"accountID": account.ID.String()}), owner)
	rec := httptest.NewRecorder()
	f.handler.ListOccurrencesForAccount(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}
