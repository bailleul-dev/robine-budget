package db

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestOperationCategorySplitRepository_ReplaceForOperationAndListForOperation(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	operation := mustCreateOperation(t, pool, account.ID, "Supermarket", "-100.00")
	groceries := createTestCategory(t, pool)
	household := createTestCategory(t, pool)
	repo := NewOperationCategorySplitRepository(pool)

	splits, err := repo.ReplaceForOperation(ctx, operation.ID, []entities.OperationCategorySplit{
		{CategoryID: groceries.ID, Amount: mustOpAmount(t, "-70.00")},
		{CategoryID: household.ID, Amount: mustOpAmount(t, "-30.00")},
	})
	if err != nil {
		t.Fatalf("ReplaceForOperation: %v", err)
	}
	// category_id has no cascade - clear the splits before the category
	// cleanups (registered after this one) run, or they'll be blocked by
	// the still-referencing split rows.
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operation_category_splits WHERE operation_id = $1", operation.ID) })
	if len(splits) != 2 {
		t.Fatalf("got %d splits, want 2", len(splits))
	}

	list, err := repo.ListForOperation(ctx, operation.ID)
	if err != nil {
		t.Fatalf("ListForOperation: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d splits, want 2", len(list))
	}
	// Highest amount first (ORDER BY amount DESC).
	if !list[0].Amount.Decimal.Equal(mustOpAmount(t, "-30.00").Decimal) {
		t.Errorf("list[0].Amount = %s, want -30.00 (DESC order)", list[0].Amount.Decimal)
	}

	// Replacing again must discard the previous splits, not append to them.
	replaced, err := repo.ReplaceForOperation(ctx, operation.ID, []entities.OperationCategorySplit{
		{CategoryID: groceries.ID, Amount: mustOpAmount(t, "-100.00")},
	})
	if err != nil {
		t.Fatalf("ReplaceForOperation (again): %v", err)
	}
	if len(replaced) != 1 {
		t.Fatalf("got %d splits, want 1 after replacing", len(replaced))
	}
	afterReplace, err := repo.ListForOperation(ctx, operation.ID)
	if err != nil {
		t.Fatalf("ListForOperation (after replace): %v", err)
	}
	if len(afterReplace) != 1 {
		t.Fatalf("got %d splits, want exactly 1 after a replace", len(afterReplace))
	}
}

func TestOperationCategorySplitRepository_ListForOperationIDs(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	op1 := mustCreateOperation(t, pool, account.ID, "Op1", "-10.00")
	op2 := mustCreateOperation(t, pool, account.ID, "Op2", "-20.00")
	category := createTestCategory(t, pool)
	repo := NewOperationCategorySplitRepository(pool)

	if _, err := repo.ReplaceForOperation(ctx, op1.ID, []entities.OperationCategorySplit{{CategoryID: category.ID, Amount: mustOpAmount(t, "-10.00")}}); err != nil {
		t.Fatalf("ReplaceForOperation op1: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operation_category_splits WHERE operation_id = $1", op1.ID) })
	if _, err := repo.ReplaceForOperation(ctx, op2.ID, []entities.OperationCategorySplit{{CategoryID: category.ID, Amount: mustOpAmount(t, "-20.00")}}); err != nil {
		t.Fatalf("ReplaceForOperation op2: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operation_category_splits WHERE operation_id = $1", op2.ID) })

	got, err := repo.ListForOperationIDs(ctx, []uuid.UUID{op1.ID, op2.ID})
	if err != nil {
		t.Fatalf("ListForOperationIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d splits, want 2", len(got))
	}
}

func TestOperationCategorySplitRepository_ListForOperationIDs_Empty(t *testing.T) {
	got, err := NewOperationCategorySplitRepository(newTestPool(t)).ListForOperationIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListForOperationIDs: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for an empty id list", got)
	}
}
