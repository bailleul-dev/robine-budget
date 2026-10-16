package db

import (
	"context"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

func mustCheckpointAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

func TestBalanceCheckpointRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	note := "opening balance"

	created, err := NewBalanceCheckpointRepository(pool).Add(ctx, &entities.BalanceCheckpoint{
		ID: newTestUUID(), AccountID: account.ID, AsOfDate: civil.New(2026, time.January, 1),
		Balance: mustCheckpointAmount(t, "1000.00"), Note: &note,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM balance_checkpoints WHERE id = $1", created.ID) })

	got, err := NewBalanceCheckpointRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if !got.Balance.Decimal.Equal(mustCheckpointAmount(t, "1000.00").Decimal) {
		t.Errorf("Balance = %s, want 1000.00", got.Balance.Decimal)
	}
	if got.Note == nil || *got.Note != note {
		t.Errorf("Note = %v, want %q", got.Note, note)
	}
}

func TestBalanceCheckpointRepository_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	got, err := NewBalanceCheckpointRepository(pool).GetByID(context.Background(), newTestUUID())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for an unknown id", got)
	}
}

func TestBalanceCheckpointRepository_ListForAccount(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewBalanceCheckpointRepository(pool)
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	dates := []civil.Date{civil.New(2026, time.January, 1), civil.New(2026, time.February, 1)}
	for _, d := range dates {
		c, err := repo.Add(ctx, &entities.BalanceCheckpoint{ID: newTestUUID(), AccountID: account.ID, AsOfDate: d, Balance: mustCheckpointAmount(t, "1.00")})
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		id := c.ID
		t.Cleanup(func() { mustExec(t, pool, "DELETE FROM balance_checkpoints WHERE id = $1", id) })
	}

	list, err := repo.ListForAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d checkpoints, want 2", len(list))
	}
	// Newest first.
	if !list[0].AsOfDate.Equal(civil.New(2026, time.February, 1)) {
		t.Errorf("list[0].AsOfDate = %s, want the most recent date first", list[0].AsOfDate)
	}
}

func TestBalanceCheckpointRepository_GetLatestOnOrBefore(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewBalanceCheckpointRepository(pool)
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	jan, err := repo.Add(ctx, &entities.BalanceCheckpoint{ID: newTestUUID(), AccountID: account.ID, AsOfDate: civil.New(2026, time.January, 1), Balance: mustCheckpointAmount(t, "100.00")})
	if err != nil {
		t.Fatalf("Add jan: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM balance_checkpoints WHERE id = $1", jan.ID) })
	mar, err := repo.Add(ctx, &entities.BalanceCheckpoint{ID: newTestUUID(), AccountID: account.ID, AsOfDate: civil.New(2026, time.March, 1), Balance: mustCheckpointAmount(t, "300.00")})
	if err != nil {
		t.Fatalf("Add mar: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM balance_checkpoints WHERE id = $1", mar.ID) })

	got, err := repo.GetLatestOnOrBefore(ctx, account.ID, civil.New(2026, time.February, 1))
	if err != nil {
		t.Fatalf("GetLatestOnOrBefore: %v", err)
	}
	if got == nil || got.ID != jan.ID {
		t.Fatalf("got %v, want the January checkpoint (latest on-or-before Feb 1)", got)
	}

	none, err := repo.GetLatestOnOrBefore(ctx, account.ID, civil.New(2025, time.December, 31))
	if err != nil {
		t.Fatalf("GetLatestOnOrBefore (none): %v", err)
	}
	if none != nil {
		t.Fatalf("got %v, want nil before any checkpoint exists", none)
	}
}

func TestBalanceCheckpointRepository_Upsert(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewBalanceCheckpointRepository(pool)
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	asOf := civil.New(2026, time.January, 1)

	first, err := repo.Upsert(ctx, &entities.BalanceCheckpoint{ID: newTestUUID(), AccountID: account.ID, AsOfDate: asOf, Balance: mustCheckpointAmount(t, "50.00")})
	if err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM balance_checkpoints WHERE id = $1", first.ID) })

	// Same account/date - the ON CONFLICT clause replaces the balance
	// rather than erroring or duplicating the row.
	second, err := repo.Upsert(ctx, &entities.BalanceCheckpoint{ID: newTestUUID(), AccountID: account.ID, AsOfDate: asOf, Balance: mustCheckpointAmount(t, "75.00")})
	if err != nil {
		t.Fatalf("Upsert (conflict): %v", err)
	}
	if !second.Balance.Decimal.Equal(mustCheckpointAmount(t, "75.00").Decimal) {
		t.Errorf("Balance = %s, want 75.00", second.Balance.Decimal)
	}

	list, err := repo.ListForAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d checkpoints, want exactly 1 after an upsert on the same date", len(list))
	}
}

func TestBalanceCheckpointRepository_Delete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	created, err := NewBalanceCheckpointRepository(pool).Add(ctx, &entities.BalanceCheckpoint{
		ID: newTestUUID(), AccountID: account.ID, AsOfDate: civil.Today(), Balance: mustCheckpointAmount(t, "1.00"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := NewBalanceCheckpointRepository(pool).Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := NewBalanceCheckpointRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil after Delete", got)
	}
}
