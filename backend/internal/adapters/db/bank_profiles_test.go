package db

import (
	"context"
	"testing"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestBankProfileRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)

	created, err := NewBankProfileRepository(pool).Add(ctx, &entities.BankProfile{
		ID: newTestUUID(), UserID: &user.ID, Name: "My Bank " + newTestUUIDString(),
		Delimiter: ",", Encoding: "utf-8", DateFormat: "2006-01-02", DecimalSeparator: ".",
		AmountMode:    entities.AmountModeDebitCredit,
		ColumnMapping: entities.ColumnMapping{Date: "Date", Label: "Label", Debit: strPtr("Debit"), Credit: strPtr("Credit")},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_profiles WHERE id = $1", created.ID) })

	got, err := NewBankProfileRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.AmountMode != entities.AmountModeDebitCredit {
		t.Errorf("AmountMode = %q, want debit_credit", got.AmountMode)
	}
	if got.ColumnMapping.Debit == nil || *got.ColumnMapping.Debit != "Debit" {
		t.Errorf("ColumnMapping.Debit = %v, want Debit", got.ColumnMapping.Debit)
	}
	if got.ColumnMapping.Amount != nil {
		t.Errorf("ColumnMapping.Amount = %v, want nil (debit/credit mode)", got.ColumnMapping.Amount)
	}
}

func TestBankProfileRepository_ListForUser_IncludesGlobalAndOwn(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewBankProfileRepository(pool)
	user := createTestUser(t, pool)
	otherUser := createTestUser(t, pool)

	global := createTestBankProfile(t, pool) // UserID nil
	mine, err := repo.Add(ctx, &entities.BankProfile{
		ID: newTestUUID(), UserID: &user.ID, Name: "Mine " + newTestUUIDString(),
		Delimiter: ";", Encoding: "utf-8", DateFormat: "02/01/2006", DecimalSeparator: ",",
		AmountMode: entities.AmountModeSingle, ColumnMapping: entities.ColumnMapping{Date: "Date", Label: "Label", Amount: strPtr("Amount")},
	})
	if err != nil {
		t.Fatalf("Add mine: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_profiles WHERE id = $1", mine.ID) })
	theirs, err := repo.Add(ctx, &entities.BankProfile{
		ID: newTestUUID(), UserID: &otherUser.ID, Name: "Theirs " + newTestUUIDString(),
		Delimiter: ";", Encoding: "utf-8", DateFormat: "02/01/2006", DecimalSeparator: ",",
		AmountMode: entities.AmountModeSingle, ColumnMapping: entities.ColumnMapping{Date: "Date", Label: "Label", Amount: strPtr("Amount")},
	})
	if err != nil {
		t.Fatalf("Add theirs: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM bank_profiles WHERE id = $1", theirs.ID) })

	list, err := repo.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	ids := map[string]bool{}
	for _, p := range list {
		ids[p.ID.String()] = true
	}
	if !ids[global.ID.String()] {
		t.Error("a global (no-owner) profile should be visible to any user")
	}
	if !ids[mine.ID.String()] {
		t.Error("a user's own profile should be visible to them")
	}
	if ids[theirs.ID.String()] {
		t.Error("another user's custom profile should not be visible")
	}
}

func TestBankProfileRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	profile := createTestBankProfile(t, pool)

	profile.Name = "Renamed " + profile.Name
	profile.AmountMode = entities.AmountModeDebitCreditSigned
	updated, err := NewBankProfileRepository(pool).Update(ctx, profile)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != profile.Name {
		t.Errorf("Name = %q, want %q", updated.Name, profile.Name)
	}
	if updated.AmountMode != entities.AmountModeDebitCreditSigned {
		t.Errorf("AmountMode = %q, want debit_credit_signed", updated.AmountMode)
	}
}
