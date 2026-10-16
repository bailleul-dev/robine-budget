package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestAccountRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	got, err := NewAccountRepository(pool).GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.UserID != user.ID {
		t.Errorf("UserID = %s, want %s", got.UserID, user.ID)
	}
	if got.Currency != "EUR" {
		t.Errorf("Currency = %q, want EUR", got.Currency)
	}
	if got.BankSyncAccountID != nil {
		t.Errorf("BankSyncAccountID = %v, want nil", got.BankSyncAccountID)
	}
}

func TestAccountRepository_ListForUser(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	otherUser := createTestUser(t, pool)

	acc1 := createTestAccount(t, pool, user.ID)
	acc2 := createTestAccount(t, pool, user.ID)
	_ = createTestAccount(t, pool, otherUser.ID) // must not show up in user's own list

	accounts, err := NewAccountRepository(pool).ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("got %d accounts, want 2", len(accounts))
	}
	ids := map[string]bool{accounts[0].ID.String(): true, accounts[1].ID.String(): true}
	if !ids[acc1.ID.String()] || !ids[acc2.ID.String()] {
		t.Fatalf("got %v, want accounts %s and %s", ids, acc1.ID, acc2.ID)
	}
}

func TestAccountRepository_GetByBankSyncAccountID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	bankSyncID := "woob-" + newTestUUIDString()
	created, err := NewAccountRepository(pool).Add(ctx, &entities.Account{
		ID: newTestUUID(), UserID: user.ID, Name: "Synced Account", Currency: "EUR", BankSyncAccountID: &bankSyncID,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM accounts WHERE id = $1", created.ID) })

	got, err := NewAccountRepository(pool).GetByBankSyncAccountID(ctx, user.ID, bankSyncID)
	if err != nil {
		t.Fatalf("GetByBankSyncAccountID: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("got %v, want account %s", got, created.ID)
	}

	// Same bank_sync_account_id, different user - the (user_id,
	// bank_sync_account_id) pair is what's unique, not the id alone (two
	// different users linking to woob accounts that happen to share an id
	// would otherwise collide, or worse, cross-match to the wrong user).
	otherUser := createTestUser(t, pool)
	miss, err := NewAccountRepository(pool).GetByBankSyncAccountID(ctx, otherUser.ID, bankSyncID)
	if err != nil {
		t.Fatalf("GetByBankSyncAccountID (other user): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil (bank_sync_account_id belongs to a different user)", miss)
	}
}

func TestAccountRepository_Delete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account, err := NewAccountRepository(pool).Add(ctx, &entities.Account{ID: newTestUUID(), UserID: user.ID, Name: "To Delete", Currency: "EUR"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := NewAccountRepository(pool).Delete(ctx, account.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := NewAccountRepository(pool).GetByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil after Delete", got)
	}
}

// Deleting an account must cascade to its operations too (migration
// 000014_cascade_account_delete) - an orphaned operation row with a
// dangling account_id would break every account-scoped query.
func TestAccountRepository_DeleteCascadesToOperations(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account, err := NewAccountRepository(pool).Add(ctx, &entities.Account{ID: newTestUUID(), UserID: user.ID, Name: "Cascade Test", Currency: "EUR"})
	if err != nil {
		t.Fatalf("Add account: %v", err)
	}
	op := mustCreateOperation(t, pool, account.ID, "Test operation", "-10.00")

	if err := NewAccountRepository(pool).Delete(ctx, account.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := NewOperationRepository(pool).GetByID(ctx, op.ID)
	if err != nil {
		t.Fatalf("GetByID after cascade delete: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want the operation gone after its account was deleted", got)
	}
}
