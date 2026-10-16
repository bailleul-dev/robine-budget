package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func TestPrevisions_OpeningBalanceSumsOperationsBeforeWindow(t *testing.T) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2025, 12, 1), Amount: amt("1000")}
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2025, 12, 15), Amount: amt("-200")}

	result, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 1), civil.New(2026, 1, 31))
	if err != nil {
		t.Fatal(err)
	}
	if !result.OpeningBalance.Decimal.Equal(amt("800").Decimal) {
		t.Errorf("opening balance = %s, want 800", result.OpeningBalance.Decimal)
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected no entries in the window, got %d", len(result.Entries))
	}
}

func TestPrevisions_AnchorsOnTheLatestCheckpointInsteadOfFullHistory(t *testing.T) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	// A stale/garbage operation from before the checkpoint must NOT be
	// summed in — that's exactly the kind of drift a checkpoint bounds.
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2020, 1, 1), Amount: amt("999999")}
	checkpoints.Add(context.Background(), &entities.BalanceCheckpoint{AccountID: accountID, AsOfDate: civil.New(2025, 12, 31), Balance: amt("500")})
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, 1, 1), Amount: amt("1000")}

	result, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 2), civil.New(2026, 1, 31))
	if err != nil {
		t.Fatal(err)
	}
	if !result.OpeningBalance.Decimal.Equal(amt("1500").Decimal) {
		t.Errorf("opening balance = %s, want 1500 (500 checkpoint + 1000, ignoring the pre-checkpoint 999999)", result.OpeningBalance.Decimal)
	}
}

// TestPrevisions_FirstCheckpointAppliesRetroactively covers the reported
// bug: importing years of operations with no checkpoint ever set produces a
// naive (and wrong) balance; adding a single checkpoint dated today must
// immediately correct the balance shown for the whole displayed window
// (which starts weeks before today), not just from today onward.
func TestPrevisions_FirstCheckpointAppliesRetroactively(t *testing.T) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	// A large, wrong-looking history — the account never had a real
	// checkpoint, so summing from an assumed opening balance of 0 drifted
	// far from the real bank balance.
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2020, 1, 1), Amount: amt("-17346.16")}
	// One real operation inside the displayed window.
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, 1, 5), Amount: amt("-30")}
	// The user just recorded their real balance today.
	checkpoints.Add(context.Background(), &entities.BalanceCheckpoint{
		ID: uuid.New(), AccountID: accountID, AsOfDate: civil.New(2026, 1, 10), Balance: amt("270"),
	})

	result, err := uc.Execute(context.Background(), accountID, civil.New(2025, 12, 11), civil.New(2026, 2, 10))
	if err != nil {
		t.Fatal(err)
	}
	// Opening balance (2025-12-10) must be back-computed from the
	// checkpoint, not from the -17346.16 legacy operation.
	if !result.OpeningBalance.Decimal.Equal(amt("300").Decimal) {
		t.Errorf("opening balance = %s, want 300 (270 checkpoint minus the -30 op still ahead of it)", result.OpeningBalance.Decimal)
	}
	if !result.ClosingBalance.Decimal.Equal(amt("270").Decimal) {
		t.Errorf("closing balance = %s, want 270", result.ClosingBalance.Decimal)
	}
	for _, e := range result.Entries {
		if e.Kind == PrevisionAdjustment {
			t.Errorf("expected no adjustment entry when the checkpoint is exactly consistent with operations, got %+v", e)
		}
	}
}

// TestPrevisions_InconsistentCheckpointInsertsCatchUpEntry covers the case
// where the account already had an earlier checkpoint, but a later one
// doesn't match what summing the operations in between would predict (a
// missed import, a bank fee, etc.) — the gap must show up as an explicit
// catch-up entry rather than silently vanish or silently override history.
func TestPrevisions_InconsistentCheckpointInsertsCatchUpEntry(t *testing.T) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	checkpoints.Add(context.Background(), &entities.BalanceCheckpoint{
		ID: uuid.New(), AccountID: accountID, AsOfDate: civil.New(2026, 1, 1), Balance: amt("1000"),
	})
	operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: civil.New(2026, 1, 10), Amount: amt("-100")}
	// A second checkpoint that doesn't match 1000 - 100 = 900 — some
	// untracked spending happened in between.
	secondID := uuid.New()
	checkpoints.Add(context.Background(), &entities.BalanceCheckpoint{
		ID: secondID, AccountID: accountID, AsOfDate: civil.New(2026, 1, 20), Balance: amt("850"),
	})

	result, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 2), civil.New(2026, 1, 31))
	if err != nil {
		t.Fatal(err)
	}

	var adjustment *PrevisionEntry
	for i := range result.Entries {
		if result.Entries[i].Kind == PrevisionAdjustment {
			adjustment = &result.Entries[i]
		}
	}
	if adjustment == nil {
		t.Fatalf("expected a catch-up adjustment entry, got entries: %+v", result.Entries)
	}
	if !adjustment.Date.Equal(civil.New(2026, 1, 20)) {
		t.Errorf("adjustment date = %s, want 2026-01-20", adjustment.Date)
	}
	if !adjustment.Amount.Decimal.Equal(amt("-50").Decimal) {
		t.Errorf("adjustment amount = %s, want -50 (850 actual vs 900 naively predicted)", adjustment.Amount.Decimal)
	}
	if adjustment.BalanceCheckpointID == nil || *adjustment.BalanceCheckpointID != secondID {
		t.Errorf("adjustment should reference the second checkpoint, got %v", adjustment.BalanceCheckpointID)
	}
	if !result.ClosingBalance.Decimal.Equal(amt("850").Decimal) {
		t.Errorf("closing balance = %s, want 850", result.ClosingBalance.Decimal)
	}
}

func TestPrevisions_ReconciledOccurrenceIsNotDoubleCounted(t *testing.T) {
	operations := newFakeOperationRepo()
	occurrences := newFakeOccurrenceRepo()
	checkpoints := newFakeCheckpointRepo()
	uc := NewPrevisionsUseCase(operations, occurrences, checkpoints)

	accountID := uuid.New()
	itemID := uuid.New()
	opID := uuid.New()
	operations.byID[opID] = &entities.Operation{ID: opID, AccountID: accountID, Date: civil.New(2026, 1, 10), Amount: amt("-800")}
	occID := uuid.New()
	occurrences.byID[occID] = &entities.PlannedOccurrence{
		ID: occID, PlannedItemID: itemID, DueDate: civil.New(2026, 1, 10), ExpectedAmount: amt("-800"),
		Status: entities.OccurrenceReconciled, ReconciledOperationID: &opID,
	}

	result, err := uc.Execute(context.Background(), accountID, civil.New(2026, 1, 1), civil.New(2026, 1, 31))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("got %d entries, want 1 (the reconciled occurrence must not add a second, forecast entry): %+v", len(result.Entries), result.Entries)
	}
	if result.Entries[0].Kind != PrevisionActual {
		t.Errorf("entry kind = %s, want actual", result.Entries[0].Kind)
	}
	if !result.ClosingBalance.Decimal.Equal(amt("-800").Decimal) {
		t.Errorf("closing balance = %s, want -800", result.ClosingBalance.Decimal)
	}
}
