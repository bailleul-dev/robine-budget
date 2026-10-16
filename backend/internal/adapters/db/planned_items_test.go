package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

func mustPlannedAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

func TestPlannedItemRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	created, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Rent"),
		Amount: mustPlannedAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", created.ID) })

	got, err := NewPlannedItemRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Label == nil || *got.Label != "Rent" || got.Kind != entities.PlannedItemKindCharge {
		t.Errorf("got %+v, unexpected", got)
	}
	if !got.Active {
		t.Error("Active = false, want true")
	}
}

func TestPlannedItemRepository_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	got, err := NewPlannedItemRepository(pool).GetByID(context.Background(), newTestUUID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for an unknown id", got)
	}
}

func TestPlannedItemRepository_ListForAccount(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewPlannedItemRepository(pool)
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	otherAccount := createTestAccount(t, pool, user.ID)

	mine, err := repo.Add(ctx, &entities.PlannedItem{ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Mine"), Amount: mustPlannedAmount(t, "1.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", mine.ID) })
	other, err := repo.Add(ctx, &entities.PlannedItem{ID: newTestUUID(), AccountID: otherAccount.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Other"), Amount: mustPlannedAmount(t, "1.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", other.ID) })

	list, err := repo.ListForAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("got %v, want only the item on account %s", list, account.ID)
	}
}

func TestPlannedItemRepository_ListForUser(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewPlannedItemRepository(pool)
	user := createTestUser(t, pool)
	otherUser := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	otherAccount := createTestAccount(t, pool, otherUser.ID)

	mine, err := repo.Add(ctx, &entities.PlannedItem{ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Mine"), Amount: mustPlannedAmount(t, "1.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", mine.ID) })
	theirs, err := repo.Add(ctx, &entities.PlannedItem{ID: newTestUUID(), AccountID: otherAccount.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Theirs"), Amount: mustPlannedAmount(t, "1.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", theirs.ID) })

	list, err := repo.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("got %v, want only %s's own item", list, user.ID)
	}
}

func TestPlannedItemRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	repo := NewPlannedItemRepository(pool)

	item, err := repo.Add(ctx, &entities.PlannedItem{ID: newTestUUID(), AccountID: account.ID, Kind: entities.PlannedItemKindCharge, Label: strPtr("Original"), Amount: mustPlannedAmount(t, "10.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	item.Label = strPtr("Updated")
	item.Active = false
	updated, err := repo.Update(ctx, item)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Label == nil || *updated.Label != "Updated" {
		t.Errorf("Label = %v, want Updated", updated.Label)
	}
	if updated.Active {
		t.Error("Active = true, want false")
	}
}
