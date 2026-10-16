package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestImportBatchRepository_AddAndListForAccount(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	profile := createTestBankProfile(t, pool)
	repo := NewImportBatchRepository(pool)

	created, err := repo.Add(ctx, &entities.ImportBatch{
		ID: newTestUUID(), AccountID: account.ID, BankProfileID: profile.ID, Filename: "releve.csv",
		RowCount: 10, ImportedCount: 8, DuplicateCount: 2, Errors: []string{"row 3: bad date"},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM import_batches WHERE id = $1", created.ID) })

	if created.Filename != "releve.csv" || created.RowCount != 10 || created.ImportedCount != 8 || created.DuplicateCount != 2 {
		t.Errorf("got %+v, unexpected", created)
	}
	if len(created.Errors) != 1 || created.Errors[0] != "row 3: bad date" {
		t.Errorf("Errors = %v, want [row 3: bad date]", created.Errors)
	}

	list, err := repo.ListForAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("got %v, want %s", list, created.ID)
	}
}

func TestImportBatchRepository_Add_NoErrors(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	profile := createTestBankProfile(t, pool)

	created, err := NewImportBatchRepository(pool).Add(ctx, &entities.ImportBatch{
		ID: newTestUUID(), AccountID: account.ID, BankProfileID: profile.ID, Filename: "clean.csv",
		RowCount: 5, ImportedCount: 5, DuplicateCount: 0,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM import_batches WHERE id = $1", created.ID) })

	if len(created.Errors) != 0 {
		t.Errorf("Errors = %v, want empty", created.Errors)
	}
}

func TestImportBatchRepository_ListForAccount_Empty(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	list, err := NewImportBatchRepository(pool).ListForAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("got %d batches, want 0", len(list))
	}
}
