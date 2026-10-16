// Use-case: project a running balance from actual operations + forecast
// occurrences, anchored on the latest balance checkpoint instead of summing
// every operation since the account's first import. Mirrors
// backend/app/domain/services/previsions.py.
package services

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

type PrevisionKind string

const (
	PrevisionActual   PrevisionKind = "actual"
	PrevisionForecast PrevisionKind = "forecast"
	// PrevisionAdjustment is a synthetic entry inserted wherever a balance
	// checkpoint's declared balance doesn't match what summing operations
	// alone would have produced — see the "retroactive anchor + catch-up"
	// logic in Execute below.
	PrevisionAdjustment PrevisionKind = "adjustment"
)

type PrevisionEntry struct {
	Date                civil.Date
	Amount              money.Amount
	Kind                PrevisionKind
	OperationID         *uuid.UUID
	PlannedOccurrenceID *uuid.UUID
	PlannedItemID       *uuid.UUID
	BalanceCheckpointID *uuid.UUID
	RunningBalance      money.Amount
}

type PrevisionsResult struct {
	OpeningBalance money.Amount
	Entries        []PrevisionEntry
	ClosingBalance money.Amount
}

type PrevisionsUseCase struct {
	operations  ports.OperationRepository
	occurrences ports.PlannedOccurrenceRepository
	checkpoints ports.BalanceCheckpointRepository
}

func NewPrevisionsUseCase(
	operations ports.OperationRepository,
	occurrences ports.PlannedOccurrenceRepository,
	checkpoints ports.BalanceCheckpointRepository,
) *PrevisionsUseCase {
	return &PrevisionsUseCase{operations: operations, occurrences: occurrences, checkpoints: checkpoints}
}

func (uc *PrevisionsUseCase) Execute(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo civil.Date) (*PrevisionsResult, error) {
	windowStart := dateFrom.AddDays(-1)

	allCheckpoints, err := uc.checkpoints.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	sort.Slice(allCheckpoints, func(i, j int) bool { return allCheckpoints[i].AsOfDate.Before(allCheckpoints[j].AsOfDate) })

	priorCheckpoint, err := uc.checkpoints.GetLatestOnOrBefore(ctx, accountID, windowStart)
	if err != nil {
		return nil, err
	}

	var anchorDate *civil.Date
	openingBalance := money.Zero()
	if priorCheckpoint != nil {
		d := priorCheckpoint.AsOfDate.AddDays(1)
		anchorDate = &d
		openingBalance = priorCheckpoint.Balance

		priorOps, err := uc.operations.ListForAccount(ctx, accountID, anchorDate, &windowStart)
		if err != nil {
			return nil, err
		}
		for _, op := range priorOps {
			openingBalance = openingBalance.Add(op.Amount)
		}
	} else if len(allCheckpoints) > 0 {
		// No checkpoint on or before the window, but at least one exists
		// later — e.g. a first-ever checkpoint dated today, with the window
		// starting 30 days back. Rather than silently ignoring it (and
		// falling back to summing every operation since the account's first
		// import, which is what produced a wildly wrong balance before this
		// existed), treat it as retroactively true: walk its balance
		// backward across the operations between the window and it.
		earliest := allCheckpoints[0]
		backOps, err := uc.operations.ListForAccount(ctx, accountID, &dateFrom, &earliest.AsOfDate)
		if err != nil {
			return nil, err
		}
		openingBalance = earliest.Balance
		for _, op := range backOps {
			openingBalance = openingBalance.Sub(op.Amount)
		}
	} else {
		// No checkpoint at all — unchanged legacy behavior: sum every
		// operation on record, from the account's very first import.
		priorOps, err := uc.operations.ListForAccount(ctx, accountID, nil, &windowStart)
		if err != nil {
			return nil, err
		}
		for _, op := range priorOps {
			openingBalance = openingBalance.Add(op.Amount)
		}
	}

	actualOps, err := uc.operations.ListForAccount(ctx, accountID, &dateFrom, &dateTo)
	if err != nil {
		return nil, err
	}
	occurrences, err := uc.occurrences.ListForAccount(ctx, accountID, &dateFrom, &dateTo)
	if err != nil {
		return nil, err
	}

	entries := make([]PrevisionEntry, 0, len(actualOps)+len(occurrences))
	for _, op := range actualOps {
		opID := op.ID
		entries = append(entries, PrevisionEntry{
			Date:        op.Date,
			Amount:      op.Amount,
			Kind:        PrevisionActual,
			OperationID: &opID,
		})
	}
	// Reconciled/skipped occurrences are already represented by their linked
	// actual operation (or intentionally excluded) — only pending forecasts
	// add a new entry.
	for _, occ := range occurrences {
		if occ.Status != entities.OccurrenceForecast {
			continue
		}
		occID := occ.ID
		itemID := occ.PlannedItemID
		entries = append(entries, PrevisionEntry{
			Date:                occ.DueDate,
			Amount:              occ.ExpectedAmount,
			Kind:                PrevisionForecast,
			PlannedOccurrenceID: &occID,
			PlannedItemID:       &itemID,
		})
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Date.Before(entries[j].Date) })

	// Any checkpoint that falls inside the window (as opposed to anchoring
	// it from before, handled above) marks a point where the account's real
	// declared balance and the running total computed from operations alone
	// may have drifted apart — e.g. an import gap, a duplicate, a bank fee
	// never imported. Snap the running balance to the checkpoint's declared
	// value there, recording the gap as an explicit catch-up entry instead
	// of silently absorbing or ignoring it.
	var midWindowCheckpoints []entities.BalanceCheckpoint
	for _, c := range allCheckpoints {
		if c.AsOfDate.After(windowStart) && !c.AsOfDate.After(dateTo) {
			midWindowCheckpoints = append(midWindowCheckpoints, c)
		}
	}

	running := openingBalance
	merged := make([]PrevisionEntry, 0, len(entries)+len(midWindowCheckpoints))
	entryIndex := 0
	for _, cp := range midWindowCheckpoints {
		for entryIndex < len(entries) && !entries[entryIndex].Date.After(cp.AsOfDate) {
			running = running.Add(entries[entryIndex].Amount)
			entries[entryIndex].RunningBalance = running
			merged = append(merged, entries[entryIndex])
			entryIndex++
		}
		if diff := cp.Balance.Sub(running); !diff.IsZero() {
			cpID := cp.ID
			running = cp.Balance
			merged = append(merged, PrevisionEntry{
				Date: cp.AsOfDate, Amount: diff, Kind: PrevisionAdjustment,
				BalanceCheckpointID: &cpID, RunningBalance: running,
			})
		}
	}
	for ; entryIndex < len(entries); entryIndex++ {
		running = running.Add(entries[entryIndex].Amount)
		entries[entryIndex].RunningBalance = running
		merged = append(merged, entries[entryIndex])
	}

	return &PrevisionsResult{OpeningBalance: openingBalance, Entries: merged, ClosingBalance: running}, nil
}
