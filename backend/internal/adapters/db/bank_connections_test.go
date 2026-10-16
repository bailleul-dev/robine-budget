package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestBankConnectionRepository_AddAndGetByUserID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewBankConnectionRepository(pool)

	created, err := repo.Add(ctx, &entities.BankConnection{
		ID: newTestUUID(), UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel,
		LoginEncrypted: []byte("encrypted-login"), PasswordEncrypted: []byte("encrypted-password"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_connections WHERE id = $1", created.ID) })

	got, err := repo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("got %v, want %s", got, created.ID)
	}
	if string(got.LoginEncrypted) != "encrypted-login" {
		t.Errorf("LoginEncrypted = %q, want encrypted-login", got.LoginEncrypted)
	}
	if got.SessionEncrypted != nil {
		t.Errorf("SessionEncrypted = %v, want nil before any sync", got.SessionEncrypted)
	}
}

func TestBankConnectionRepository_GetByUserID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	user := createTestUser(t, pool)

	got, err := NewBankConnectionRepository(pool).GetByUserID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if got != nil {
		t.Fatalf("got %v, want nil for a user with no bank connection", got)
	}
}

func TestBankConnectionRepository_UpdateSession(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewBankConnectionRepository(pool)

	created, err := repo.Add(ctx, &entities.BankConnection{
		ID: newTestUUID(), UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_connections WHERE id = $1", created.ID) })

	if err := repo.UpdateSession(ctx, created.ID, []byte("new-session")); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	got, err := repo.GetByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if string(got.SessionEncrypted) != "new-session" {
		t.Errorf("SessionEncrypted = %q, want new-session", got.SessionEncrypted)
	}
}

func TestBankConnectionRepository_ListUserIDs(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	repo := NewBankConnectionRepository(pool)

	created, err := repo.Add(ctx, &entities.BankConnection{
		ID: newTestUUID(), UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_connections WHERE id = $1", created.ID) })

	ids, err := repo.ListUserIDs(ctx)
	if err != nil {
		t.Fatalf("ListUserIDs: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == user.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("ListUserIDs = %v, want it to include %s", ids, user.ID)
	}
}
