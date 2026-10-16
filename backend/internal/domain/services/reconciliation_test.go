package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

func newTestReconcileUseCase(operations *fakeOperationRepo, occurrences *fakeOccurrenceRepo, plannedItems *fakePlannedItemRepo) *ReconcileUseCase {
	return NewReconcileUseCase(operations, occurrences, plannedItems)
}

func reconciliationAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

// --- Reconcile / Unreconcile --------------------------------------------

func TestReconcile_HappyPath(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "800.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "RENT", Amount: reconciliationAmount(t, "-800.00")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	updated, err := uc.Reconcile(ctx, occ.ID, op.ID)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if updated.Status != entities.OccurrenceReconciled {
		t.Fatalf("Status = %s, want reconciled", updated.Status)
	}
	if updated.ReconciledOperationID == nil || *updated.ReconciledOperationID != op.ID {
		t.Fatalf("ReconciledOperationID = %v, want %s", updated.ReconciledOperationID, op.ID)
	}
	if updated.ReconciledAmount == nil || !updated.ReconciledAmount.Decimal.Equal(op.Amount.Decimal) {
		t.Fatalf("ReconciledAmount = %v, want %s", updated.ReconciledAmount, op.Amount.Decimal)
	}
}

func TestReconcile_OccurrenceNotFound(t *testing.T) {
	ctx := context.Background()
	uc := newTestReconcileUseCase(newFakeOperationRepo(), newFakeOccurrenceRepo(), newFakePlannedItemRepo())
	if _, err := uc.Reconcile(ctx, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected an error for a missing occurrence")
	}
}

func TestReconcile_OperationNotFound(t *testing.T) {
	ctx := context.Background()
	occurrences := newFakeOccurrenceRepo()
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: uuid.New(), DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "10.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ

	uc := newTestReconcileUseCase(newFakeOperationRepo(), occurrences, newFakePlannedItemRepo())
	if _, err := uc.Reconcile(ctx, occ.ID, uuid.New()); err == nil {
		t.Fatal("expected an error for a missing operation")
	}
}

// Reconciling should carry the planned item's category onto the operation
// when it doesn't already have one of its own.
func TestReconcile_AppliesItemCategoryWhenOperationHasNone(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()
	categoryID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Rent"), CategoryID: &categoryID, MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "800.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "RENT", Amount: reconciliationAmount(t, "-800.00")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	if _, err := uc.Reconcile(ctx, occ.ID, op.ID); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := operations.byID[op.ID].CategoryID; got == nil || *got != categoryID {
		t.Fatalf("operation CategoryID = %v, want %s", got, categoryID)
	}
}

// A category the user already set on the operation by hand must never be
// silently overwritten by the recurring item's own category.
func TestReconcile_DoesNotOverwriteExistingCategory(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()
	itemCategoryID := uuid.New()
	userCategoryID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Rent"), CategoryID: &itemCategoryID, MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "800.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "RENT", Amount: reconciliationAmount(t, "-800.00"), CategoryID: &userCategoryID}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	if _, err := uc.Reconcile(ctx, occ.ID, op.ID); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := operations.byID[op.ID].CategoryID; got == nil || *got != userCategoryID {
		t.Fatalf("operation CategoryID = %v, want the user's own choice %s to survive", got, userCategoryID)
	}
}

func TestUnreconcile_HappyPath(t *testing.T) {
	ctx := context.Background()
	occurrences := newFakeOccurrenceRepo()
	opID := uuid.New()
	amount := reconciliationAmount(t, "800.00")
	occ := &entities.PlannedOccurrence{
		ID: uuid.New(), PlannedItemID: uuid.New(), DueDate: civil.Today(),
		ExpectedAmount: amount, Status: entities.OccurrenceReconciled,
		ReconciledOperationID: &opID, ReconciledAmount: &amount,
	}
	occurrences.byID[occ.ID] = occ

	uc := newTestReconcileUseCase(newFakeOperationRepo(), occurrences, newFakePlannedItemRepo())
	updated, err := uc.Unreconcile(ctx, occ.ID)
	if err != nil {
		t.Fatalf("Unreconcile: %v", err)
	}
	if updated.Status != entities.OccurrenceForecast {
		t.Fatalf("Status = %s, want forecast", updated.Status)
	}
	if updated.ReconciledOperationID != nil {
		t.Fatalf("ReconciledOperationID = %v, want nil", updated.ReconciledOperationID)
	}
	if updated.ReconciledAmount != nil {
		t.Fatalf("ReconciledAmount = %v, want nil", updated.ReconciledAmount)
	}
}

func TestUnreconcile_NotFound(t *testing.T) {
	ctx := context.Background()
	uc := newTestReconcileUseCase(newFakeOperationRepo(), newFakeOccurrenceRepo(), newFakePlannedItemRepo())
	if _, err := uc.Unreconcile(ctx, uuid.New()); err == nil {
		t.Fatal("expected an error for a missing occurrence")
	}
}

// --- SuggestMatches ------------------------------------------------------

func TestSuggestMatches_MatchesByAmountAndDate(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	accountID := uuid.New()
	itemID := uuid.New()
	occurrences.itemAccount[itemID] = accountID

	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 5), ExpectedAmount: reconciliationAmount(t, "-42.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 6), Amount: reconciliationAmount(t, "-42.00")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, accountID, 0)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(suggestions))
	}
	if suggestions[0].Operation.ID != op.ID || suggestions[0].Occurrence.ID != occ.ID {
		t.Fatalf("got %+v, want a match between op %s and occurrence %s", suggestions[0], op.ID, occ.ID)
	}
}

func TestSuggestMatches_NoMatchWhenAmountDiffers(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	accountID := uuid.New()
	itemID := uuid.New()
	occurrences.itemAccount[itemID] = accountID

	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 5), ExpectedAmount: reconciliationAmount(t, "-42.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 5), Amount: reconciliationAmount(t, "-42.50")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, accountID, 0)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("got %d suggestions, want 0 (amounts differ)", len(suggestions))
	}
}

func TestSuggestMatches_OutsideToleranceIsIgnored(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	accountID := uuid.New()
	itemID := uuid.New()
	occurrences.itemAccount[itemID] = accountID

	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 1), ExpectedAmount: reconciliationAmount(t, "-42.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	// 10 days away, outside a 5-day tolerance.
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 11), Amount: reconciliationAmount(t, "-42.00")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, accountID, 5)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("got %d suggestions, want 0 (outside tolerance)", len(suggestions))
	}
}

// Two occurrences share the same amount; two candidate operations exist.
// Each occurrence should get its own closest-by-date operation, and no
// operation should ever be suggested for more than one occurrence.
func TestSuggestMatches_DoesNotReuseAnOperation(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	accountID := uuid.New()
	itemID := uuid.New()
	occurrences.itemAccount[itemID] = accountID
	amount := reconciliationAmount(t, "-15.00")

	occ1 := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 1), ExpectedAmount: amount, Status: entities.OccurrenceForecast}
	occ2 := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 8), ExpectedAmount: amount, Status: entities.OccurrenceForecast}
	occurrences.byID[occ1.ID] = occ1
	occurrences.byID[occ2.ID] = occ2

	op1 := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 1), Amount: amount}
	op2 := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 8), Amount: amount}
	operations.byID[op1.ID] = op1
	operations.byID[op2.ID] = op2

	uc := newTestReconcileUseCase(operations, occurrences, newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, accountID, 5)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("got %d suggestions, want 2", len(suggestions))
	}
	usedOps := map[uuid.UUID]int{}
	for _, s := range suggestions {
		usedOps[s.Operation.ID]++
	}
	for opID, n := range usedOps {
		if n > 1 {
			t.Fatalf("operation %s was suggested %d times, want at most 1", opID, n)
		}
	}
}

// An operation already reconciled to some occurrence (excluded from
// ListUnreconciled) must never show up as a suggestion for another one.
func TestSuggestMatches_ExcludesAlreadyReconciledOperations(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	accountID := uuid.New()
	itemID := uuid.New()
	occurrences.itemAccount[itemID] = accountID

	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: itemID, DueDate: civil.New(2026, time.March, 5), ExpectedAmount: reconciliationAmount(t, "-42.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 5), Amount: reconciliationAmount(t, "-42.00")}
	operations.byID[op.ID] = op
	operations.reconciledIDs[op.ID] = true

	uc := newTestReconcileUseCase(operations, occurrences, newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, accountID, 5)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("got %d suggestions, want 0 (operation already reconciled)", len(suggestions))
	}
}

func TestSuggestMatches_NoUnreconciledOperations(t *testing.T) {
	ctx := context.Background()
	uc := newTestReconcileUseCase(newFakeOperationRepo(), newFakeOccurrenceRepo(), newFakePlannedItemRepo())
	suggestions, err := uc.SuggestMatches(ctx, uuid.New(), 5)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if suggestions != nil {
		t.Fatalf("got %v, want nil", suggestions)
	}
}

// --- AutoMatchByLabel ------------------------------------------------------

func TestAutoMatchByLabel_FillsPendingForecastOccurrence(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Netflix"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.New(2026, time.March, 5), ExpectedAmount: reconciliationAmount(t, "-15.49"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	// Label matches case-insensitively, date within tolerance.
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 6), Label: "netflix", Amount: reconciliationAmount(t, "-15.49")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if len(reconciled) != 1 {
		t.Fatalf("got %d reconciled, want 1", len(reconciled))
	}
	if reconciled[0].ID != occ.ID {
		t.Fatalf("reconciled occurrence %s, want the existing forecast %s", reconciled[0].ID, occ.ID)
	}
	if reconciled[0].Status != entities.OccurrenceReconciled {
		t.Fatalf("Status = %s, want reconciled", reconciled[0].Status)
	}
	if reconciled[0].ReconciledOperationID == nil || *reconciled[0].ReconciledOperationID != op.ID {
		t.Fatalf("ReconciledOperationID = %v, want %s", reconciled[0].ReconciledOperationID, op.ID)
	}
}

// No forecast occurrence exists yet (the operation predates the item's
// first generated occurrence) — a new one should be backfilled straight in
// "reconciled" status, dated at the operation's own date.
func TestAutoMatchByLabel_BackfillsPastOperationWithNoOccurrence(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Netflix"), Kind: entities.PlannedItemKindCharge, Amount: reconciliationAmount(t, "15.49"), MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2025, time.January, 10), Label: "Netflix", Amount: reconciliationAmount(t, "-15.49")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if len(reconciled) != 1 {
		t.Fatalf("got %d reconciled, want 1", len(reconciled))
	}
	backfilled := reconciled[0]
	if backfilled.Status != entities.OccurrenceReconciled {
		t.Fatalf("Status = %s, want reconciled", backfilled.Status)
	}
	if !backfilled.DueDate.Equal(op.Date) {
		t.Fatalf("DueDate = %s, want the operation's own date %s", backfilled.DueDate, op.Date)
	}
	if backfilled.ReconciledOperationID == nil || *backfilled.ReconciledOperationID != op.ID {
		t.Fatalf("ReconciledOperationID = %v, want %s", backfilled.ReconciledOperationID, op.ID)
	}
}

// A charge item's expected amount (stored positive on the item) must be
// compared against the operation's own negative sign convention — a charge
// operation whose amount doesn't match (in absolute terms) the item's
// amount must not be backfilled.
func TestAutoMatchByLabel_BackfillSkipsWhenAmountDiffers(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Netflix"), Kind: entities.PlannedItemKindCharge, Amount: reconciliationAmount(t, "15.49"), MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2025, time.January, 10), Label: "Netflix", Amount: reconciliationAmount(t, "-9.99")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if len(reconciled) != 0 {
		t.Fatalf("got %d reconciled, want 0 (amount mismatch)", len(reconciled))
	}
	if operations.byID[op.ID].CategoryID != nil {
		t.Fatal("operation should not have been touched at all")
	}
}

// Running AutoMatchByLabel twice over the same backlog must not create a
// second occurrence for the same (item, date) pair.
func TestAutoMatchByLabel_DoesNotDuplicateBackfilledOccurrence(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Netflix"), Kind: entities.PlannedItemKindCharge, Amount: reconciliationAmount(t, "15.49"), MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2025, time.January, 10), Label: "Netflix", Amount: reconciliationAmount(t, "-15.49")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	first, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("first AutoMatchByLabel: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first run: got %d reconciled, want 1", len(first))
	}

	// The operation is now reconciled (linked from the backfilled
	// occurrence) - mark it so ListUnreconciled excludes it, matching what
	// the real DB-backed repo would report on a second run.
	operations.reconciledIDs[op.ID] = true

	second, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("second AutoMatchByLabel: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second run: got %d reconciled, want 0 (operation already reconciled)", len(second))
	}
	if len(occurrences.byID) != 1 {
		t.Fatalf("got %d occurrences stored, want exactly 1", len(occurrences.byID))
	}
}

func TestAutoMatchByLabel_NoMatchingLabelIsSkipped(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Netflix"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel}
	plannedItems.byID[item.ID] = item
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "Some Random Shop", Amount: reconciliationAmount(t, "-15.49")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if len(reconciled) != 0 {
		t.Fatalf("got %d reconciled, want 0 (no label match)", len(reconciled))
	}
}

func TestAutoMatchByLabel_NoPlannedItemsIsANoop(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	accountID := uuid.New()
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "Netflix", Amount: reconciliationAmount(t, "-15.49")}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, newFakeOccurrenceRepo(), newFakePlannedItemRepo())
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if reconciled != nil {
		t.Fatalf("got %v, want nil", reconciled)
	}
}

func TestAutoMatchByLabel_NoUnreconciledOperationsIsANoop(t *testing.T) {
	ctx := context.Background()
	uc := newTestReconcileUseCase(newFakeOperationRepo(), newFakeOccurrenceRepo(), newFakePlannedItemRepo())
	reconciled, err := uc.AutoMatchByLabel(ctx, uuid.New())
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if reconciled != nil {
		t.Fatalf("got %v, want nil", reconciled)
	}
}

// --- Category-mode budgets --------------------------------------------

func TestCategoryBudgetActuals_SumsOperationsWithinEachPeriod(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()
	groceriesID := uuid.New()
	otherCategoryID := uuid.New()

	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: accountID, Kind: entities.PlannedItemKindCharge,
		MatchMode: entities.PlannedItemMatchModeCategory, CategoryID: &groceriesID,
		Amount: reconciliationAmount(t, "400.00"), StartDate: civil.New(2026, time.January, 1),
	}
	plannedItems.byID[item.ID] = item

	jan := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.New(2026, time.January, 31), ExpectedAmount: reconciliationAmount(t, "-400.00"), Status: entities.OccurrenceForecast}
	feb := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.New(2026, time.February, 28), ExpectedAmount: reconciliationAmount(t, "-400.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[jan.ID] = jan
	occurrences.byID[feb.ID] = feb

	// Two January groceries runs, one February one, one in a different
	// category (must be excluded), one in March (outside both periods,
	// must be excluded).
	ops := []*entities.Operation{
		{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.January, 5), Label: "Carrefour", Amount: reconciliationAmount(t, "-45.20"), CategoryID: &groceriesID},
		{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.January, 20), Label: "Monoprix", Amount: reconciliationAmount(t, "-62.10"), CategoryID: &groceriesID},
		{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.February, 3), Label: "Boulangerie", Amount: reconciliationAmount(t, "-8.50"), CategoryID: &groceriesID},
		{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.January, 10), Label: "Cinema", Amount: reconciliationAmount(t, "-12.00"), CategoryID: &otherCategoryID},
		{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, time.March, 2), Label: "Carrefour", Amount: reconciliationAmount(t, "-30.00"), CategoryID: &groceriesID},
	}
	for _, op := range ops {
		operations.byID[op.ID] = op
	}

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	actuals, err := uc.CategoryBudgetActuals(ctx, accountID, []entities.PlannedOccurrence{*jan, *feb})
	if err != nil {
		t.Fatalf("CategoryBudgetActuals: %v", err)
	}

	if got := actuals[jan.ID]; !got.Decimal.Equal(reconciliationAmount(t, "-107.30").Decimal) {
		t.Errorf("January actual = %s, want -107.30 (45.20 + 62.10, excluding the other category)", got.Decimal)
	}
	if got := actuals[feb.ID]; !got.Decimal.Equal(reconciliationAmount(t, "-8.50").Decimal) {
		t.Errorf("February actual = %s, want -8.50 (March's operation must not leak in)", got.Decimal)
	}
}

func TestCategoryBudgetActuals_OmitsLabelModeItems(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	item := &entities.PlannedItem{ID: uuid.New(), AccountID: accountID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge, MatchMode: entities.PlannedItemMatchModeLabel, StartDate: civil.Today()}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "-800.00"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	actuals, err := uc.CategoryBudgetActuals(ctx, accountID, []entities.PlannedOccurrence{*occ})
	if err != nil {
		t.Fatalf("CategoryBudgetActuals: %v", err)
	}
	if _, ok := actuals[occ.ID]; ok {
		t.Errorf("a label-mode item's occurrence should be omitted from the result entirely, got an entry: %v", actuals[occ.ID])
	}
}

func TestSuggestMatches_ExcludesCategoryModeOccurrences(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()
	groceriesID := uuid.New()

	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: accountID, Kind: entities.PlannedItemKindCharge,
		MatchMode: entities.PlannedItemMatchModeCategory, CategoryID: &groceriesID, StartDate: civil.Today(),
	}
	plannedItems.byID[item.ID] = item
	occ := &entities.PlannedOccurrence{ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: reconciliationAmount(t, "-45.20"), Status: entities.OccurrenceForecast}
	occurrences.byID[occ.ID] = occ
	occurrences.itemAccount[item.ID] = accountID

	// Same amount and date as the occurrence — would suggest a match if
	// category-mode occurrences weren't excluded.
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "Carrefour", Amount: reconciliationAmount(t, "-45.20"), CategoryID: &groceriesID}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	suggestions, err := uc.SuggestMatches(ctx, accountID, 0)
	if err != nil {
		t.Fatalf("SuggestMatches: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("got %v, want no suggestions — a category budget's occurrences aggregate, they don't get 1-to-1 suggestions", suggestions)
	}
}

func TestAutoMatchByLabel_IgnoresCategoryModeItemsWithNoLabel(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()
	groceriesID := uuid.New()

	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: accountID, Kind: entities.PlannedItemKindCharge,
		MatchMode: entities.PlannedItemMatchModeCategory, CategoryID: &groceriesID, StartDate: civil.Today(),
	}
	plannedItems.byID[item.ID] = item

	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.Today(), Label: "Carrefour", Amount: reconciliationAmount(t, "-45.20"), CategoryID: &groceriesID}
	operations.byID[op.ID] = op

	uc := newTestReconcileUseCase(operations, occurrences, plannedItems)
	// Must not panic dereferencing a nil Label, and must not match/reconcile
	// anything for a category-mode item (it has no label to match by).
	reconciled, err := uc.AutoMatchByLabel(ctx, accountID)
	if err != nil {
		t.Fatalf("AutoMatchByLabel: %v", err)
	}
	if reconciled != nil {
		t.Fatalf("got %v, want nil", reconciled)
	}
}

// --- helpers used only here -----------------------------------------------

func TestDaysBetween(t *testing.T) {
	a := civil.New(2026, time.March, 1)
	b := civil.New(2026, time.March, 6)
	if got := daysBetween(a, b); got != 5 {
		t.Fatalf("daysBetween(a, b) = %d, want 5", got)
	}
	if got := daysBetween(b, a); got != 5 {
		t.Fatalf("daysBetween(b, a) = %d, want 5 (symmetric)", got)
	}
	if got := daysBetween(a, a); got != 0 {
		t.Fatalf("daysBetween(a, a) = %d, want 0", got)
	}
}

func TestItemSignedAmount(t *testing.T) {
	charge := entities.PlannedItem{Kind: entities.PlannedItemKindCharge, Amount: reconciliationAmount(t, "50.00")}
	if got := itemSignedAmount(charge); !got.Decimal.Equal(reconciliationAmount(t, "-50.00").Decimal) {
		t.Fatalf("charge signed amount = %s, want -50.00", got.Decimal)
	}

	income := entities.PlannedItem{Kind: entities.PlannedItemKindIncome, Amount: reconciliationAmount(t, "50.00")}
	if got := itemSignedAmount(income); !got.Decimal.Equal(reconciliationAmount(t, "50.00").Decimal) {
		t.Fatalf("income signed amount = %s, want 50.00", got.Decimal)
	}
}
