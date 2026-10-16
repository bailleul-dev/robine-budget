package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func TestComputeInitialBalance_AnchorsBeforeTheFirstOperation(t *testing.T) {
	operations := newFakeOperationRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewComputeInitialBalanceUseCase(operations, checkpoints)

	accountID := uuid.New()
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2020, 1, 1), Amount: amt("-17346.16")}
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, 1, 5), Amount: amt("-30")}

	created, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 10), amt("270"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created.AsOfDate.Equal(civil.New(2019, 12, 31)) {
		t.Errorf("as_of_date = %s, want 2019-12-31 (the day before the earliest operation)", created.AsOfDate)
	}
	if !created.Balance.Decimal.Equal(amt("17646.16").Decimal) {
		t.Errorf("balance = %s, want 17646.16 (270 minus -17346.16 minus -30)", created.Balance.Decimal)
	}
}

func TestComputeInitialBalance_NoOperationsUsesGivenDateDirectly(t *testing.T) {
	operations := newFakeOperationRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewComputeInitialBalanceUseCase(operations, checkpoints)

	accountID := uuid.New()
	created, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 10), amt("270"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created.AsOfDate.Equal(civil.New(2026, 1, 10)) {
		t.Errorf("as_of_date = %s, want 2026-01-10 (no history to anchor before)", created.AsOfDate)
	}
	if !created.Balance.Decimal.Equal(amt("270").Decimal) {
		t.Errorf("balance = %s, want 270 unchanged", created.Balance.Decimal)
	}
}

func TestComputeInitialBalance_RefusesWhenACheckpointAlreadyExists(t *testing.T) {
	operations := newFakeOperationRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewComputeInitialBalanceUseCase(operations, checkpoints)

	accountID := uuid.New()
	checkpoints.Add(context.Background(), &entities.BalanceCheckpoint{
		ID: uuid.New(), AccountID: accountID, AsOfDate: civil.New(2025, 12, 31), Balance: amt("500"),
	})

	_, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 10), amt("270"), nil)
	if !errors.Is(err, ErrInitialBalanceAlreadySet) {
		t.Fatalf("expected ErrInitialBalanceAlreadySet, got %v", err)
	}
}

func TestComputeInitialBalance_MatchesPrevisionsRetroactiveComputation(t *testing.T) {
	// The whole point of this use-case: once its checkpoint is stored,
	// PrevisionsUseCase's plain "checkpoint on or before the window" path
	// (no retroactive special-casing needed) must reproduce exactly the
	// same opening balance previsions already computed retroactively for
	// this same scenario (see TestPrevisions_FirstCheckpointAppliesRetroactively).
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	initialBalanceUC := NewComputeInitialBalanceUseCase(operations, checkpoints)
	previsionsUC := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2020, 1, 1), Amount: amt("-17346.16")}
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, 1, 5), Amount: amt("-30")}

	if _, err := initialBalanceUC.Execute(context.Background(), accountID, civil.New(2026, 1, 10), amt("270"), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := previsionsUC.Execute(context.Background(), accountID, civil.New(2025, 12, 11), civil.New(2026, 2, 10))
	if err != nil {
		t.Fatal(err)
	}
	if !result.OpeningBalance.Decimal.Equal(amt("300").Decimal) {
		t.Errorf("opening balance = %s, want 300 (matching the retroactive-computation test)", result.OpeningBalance.Decimal)
	}
	if !result.ClosingBalance.Decimal.Equal(amt("270").Decimal) {
		t.Errorf("closing balance = %s, want 270", result.ClosingBalance.Decimal)
	}
}
