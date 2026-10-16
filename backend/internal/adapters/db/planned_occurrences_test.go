package db

import (
	"context"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func TestPlannedOccurrenceRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	created, err := NewPlannedOccurrenceRepository(pool).Add(ctx, &entities.PlannedOccurrence{
		ID: newTestUUID(), PlannedItemID: item.ID, DueDate: civil.Today(),
		ExpectedAmount: mustPlannedAmount(t, "-800.00"), Status: entities.OccurrenceReconciled,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := NewPlannedOccurrenceRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.PlannedItemID != item.ID {
		t.Fatalf("got %v, want occurrence for item %s", got, item.ID)
	}
}

func TestPlannedOccurrenceRepository_ListForItem(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	repo := NewPlannedOccurrenceRepository(pool)
	occs, err := repo.AddMany(ctx, []entities.PlannedOccurrence{
		{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: civil.New(2026, time.January, 1), ExpectedAmount: mustPlannedAmount(t, "-800.00"), Status: entities.OccurrenceReconciled},
		{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: civil.New(2026, time.February, 1), ExpectedAmount: mustPlannedAmount(t, "-800.00"), Status: entities.OccurrenceReconciled},
	})
	if err != nil {
		t.Fatalf("AddMany: %v", err)
	}
	if len(occs) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(occs))
	}

	list, err := repo.ListForItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListForItem: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(list))
	}
}

func TestPlannedOccurrenceRepository_ListForAccount_DateFilter(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	repo := NewPlannedOccurrenceRepository(pool)
	jan := civil.New(2026, time.January, 1)
	feb := civil.New(2026, time.February, 1)
	mar := civil.New(2026, time.March, 1)
	if _, err := repo.AddMany(ctx, []entities.PlannedOccurrence{
		{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: jan, ExpectedAmount: mustPlannedAmount(t, "-1.00"), Status: entities.OccurrenceReconciled},
		{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: feb, ExpectedAmount: mustPlannedAmount(t, "-1.00"), Status: entities.OccurrenceReconciled},
		{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: mar, ExpectedAmount: mustPlannedAmount(t, "-1.00"), Status: entities.OccurrenceReconciled},
	}); err != nil {
		t.Fatalf("AddMany: %v", err)
	}

	all, err := repo.ListForAccount(ctx, account.ID, nil, nil)
	if err != nil {
		t.Fatalf("ListForAccount (unbounded): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d occurrences, want 3", len(all))
	}

	febToMar, err := repo.ListForAccount(ctx, account.ID, &feb, &mar)
	if err != nil {
		t.Fatalf("ListForAccount (feb-mar): %v", err)
	}
	if len(febToMar) != 2 {
		t.Fatalf("got %d occurrences in [feb, mar], want 2", len(febToMar))
	}
}

func TestPlannedOccurrenceRepository_GetByItemAndDate(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	repo := NewPlannedOccurrenceRepository(pool)
	due := civil.New(2026, time.January, 1)
	created, err := repo.Add(ctx, &entities.PlannedOccurrence{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: due, ExpectedAmount: mustPlannedAmount(t, "-1.00"), Status: entities.OccurrenceReconciled})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := repo.GetByItemAndDate(ctx, item.ID, due)
	if err != nil {
		t.Fatalf("GetByItemAndDate: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("got %v, want %s", got, created.ID)
	}

	miss, err := repo.GetByItemAndDate(ctx, item.ID, civil.New(2026, time.December, 25))
	if err != nil {
		t.Fatalf("GetByItemAndDate (miss): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil for an unscheduled date", miss)
	}
}

func TestPlannedOccurrenceRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })
	operation := mustCreateOperation(t, pool, account.ID, "Rent payment", "-800.00")

	repo := NewPlannedOccurrenceRepository(pool)
	occ, err := repo.Add(ctx, &entities.PlannedOccurrence{ID: newTestUUID(), PlannedItemID: item.ID, DueDate: civil.Today(), ExpectedAmount: mustPlannedAmount(t, "-800.00"), Status: entities.OccurrenceForecast})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	occ.Status = entities.OccurrenceReconciled
	occ.ReconciledOperationID = &operation.ID
	reconciledAmount := mustPlannedAmount(t, "-800.00")
	occ.ReconciledAmount = &reconciledAmount
	updated, err := repo.Update(ctx, occ)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != entities.OccurrenceReconciled {
		t.Errorf("Status = %q, want reconciled", updated.Status)
	}
	if updated.ReconciledOperationID == nil || *updated.ReconciledOperationID != operation.ID {
		t.Errorf("ReconciledOperationID = %v, want %s", updated.ReconciledOperationID, operation.ID)
	}
}
