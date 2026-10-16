// Use-case: link (or unlink) actual Operations against forecast
// PlannedOccurrences. Mirrors backend/app/domain/services/reconciliation.py.
package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

const defaultDateToleranceDays = 5

type ReconciliationSuggestion struct {
	Occurrence entities.PlannedOccurrence
	Operation  entities.Operation
}

type ReconcileUseCase struct {
	operations   ports.OperationRepository
	occurrences  ports.PlannedOccurrenceRepository
	plannedItems ports.PlannedItemRepository
}

func NewReconcileUseCase(
	operations ports.OperationRepository,
	occurrences ports.PlannedOccurrenceRepository,
	plannedItems ports.PlannedItemRepository,
) *ReconcileUseCase {
	return &ReconcileUseCase{operations: operations, occurrences: occurrences, plannedItems: plannedItems}
}

func (uc *ReconcileUseCase) Reconcile(ctx context.Context, occurrenceID, operationID uuid.UUID) (*entities.PlannedOccurrence, error) {
	occurrence, err := uc.occurrences.GetByID(ctx, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil {
		return nil, fmt.Errorf("planned occurrence %s not found", occurrenceID)
	}
	operation, err := uc.operations.GetByID(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if operation == nil {
		return nil, fmt.Errorf("operation %s not found", operationID)
	}

	if err := uc.applyItemCategory(ctx, occurrence.PlannedItemID, operation); err != nil {
		return nil, err
	}

	occurrence.Status = entities.OccurrenceReconciled
	occurrence.ReconciledOperationID = &operation.ID
	amount := operation.Amount
	occurrence.ReconciledAmount = &amount
	return uc.occurrences.Update(ctx, occurrence)
}

// applyItemCategory carries a planned item's category onto an operation
// being reconciled against one of its occurrences, if the operation doesn't
// already have one of its own — recurring items are categorized once, and
// every operation that fulfills one inherits it instead of needing manual
// re-categorization every month.
func (uc *ReconcileUseCase) applyItemCategory(ctx context.Context, plannedItemID uuid.UUID, operation *entities.Operation) error {
	if operation.CategoryID != nil {
		return nil
	}
	item, err := uc.plannedItems.GetByID(ctx, plannedItemID)
	if err != nil {
		return err
	}
	if item == nil || item.CategoryID == nil {
		return nil
	}
	operation.CategoryID = item.CategoryID
	_, err = uc.operations.Update(ctx, operation)
	return err
}

func (uc *ReconcileUseCase) Unreconcile(ctx context.Context, occurrenceID uuid.UUID) (*entities.PlannedOccurrence, error) {
	occurrence, err := uc.occurrences.GetByID(ctx, occurrenceID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil {
		return nil, fmt.Errorf("planned occurrence %s not found", occurrenceID)
	}
	occurrence.Status = entities.OccurrenceForecast
	occurrence.ReconciledOperationID = nil
	occurrence.ReconciledAmount = nil
	return uc.occurrences.Update(ctx, occurrence)
}

// SuggestMatches: naive suggestions — exact amount match within a date
// window. Never auto-applies.
func (uc *ReconcileUseCase) SuggestMatches(ctx context.Context, accountID uuid.UUID, dateToleranceDays int) ([]ReconciliationSuggestion, error) {
	if dateToleranceDays <= 0 {
		dateToleranceDays = defaultDateToleranceDays
	}
	unreconciledOps, err := uc.operations.ListUnreconciled(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(unreconciledOps) == 0 {
		return nil, nil
	}

	// Category-mode occurrences never get a single-operation match — every
	// operation in their category is summed into them instead (see
	// CategoryBudgetActuals) — so they must never appear as a suggested 1-to-1
	// match here.
	items, err := uc.plannedItems.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	categoryModeItems := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if item.MatchMode == entities.PlannedItemMatchModeCategory {
			categoryModeItems[item.ID] = true
		}
	}

	windowStart := unreconciledOps[0].Date
	windowEnd := unreconciledOps[0].Date
	for _, op := range unreconciledOps {
		if op.Date.Before(windowStart) {
			windowStart = op.Date
		}
		if op.Date.After(windowEnd) {
			windowEnd = op.Date
		}
	}
	windowStart = windowStart.AddDays(-dateToleranceDays)
	windowEnd = windowEnd.AddDays(dateToleranceDays)

	allOccurrences, err := uc.occurrences.ListForAccount(ctx, accountID, &windowStart, &windowEnd)
	if err != nil {
		return nil, err
	}
	var pending []entities.PlannedOccurrence
	for _, occ := range allOccurrences {
		if occ.Status == entities.OccurrenceForecast && !categoryModeItems[occ.PlannedItemID] {
			pending = append(pending, occ)
		}
	}

	var suggestions []ReconciliationSuggestion
	used := map[uuid.UUID]bool{}
	for _, occurrence := range pending {
		var best *entities.Operation
		bestDelta := -1
		for i := range unreconciledOps {
			op := unreconciledOps[i]
			if used[op.ID] {
				continue
			}
			if !op.Amount.Decimal.Equal(occurrence.ExpectedAmount.Decimal) {
				continue
			}
			delta := daysBetween(op.Date, occurrence.DueDate)
			if delta > dateToleranceDays {
				continue
			}
			if best == nil || delta < bestDelta {
				best, bestDelta = &unreconciledOps[i], delta
			}
		}
		if best != nil {
			suggestions = append(suggestions, ReconciliationSuggestion{Occurrence: occurrence, Operation: *best})
			used[best.ID] = true
		}
	}

	return suggestions, nil
}

func daysBetween(a, b civil.Date) int {
	hours := a.Time.Sub(b.Time).Hours()
	return int(math.Round(math.Abs(hours) / 24))
}

func normalizeLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

// itemSignedAmount matches Operation.Amount's convention (positive=income,
// negative=expense) — see GenerateOccurrencesUseCase.Execute.
func itemSignedAmount(item entities.PlannedItem) money.Amount {
	if item.Kind != entities.PlannedItemKindIncome {
		return item.Amount.Neg()
	}
	return item.Amount
}

// AutoMatchByLabel reconciles unreconciled operations straight away — no
// suggestion/confirmation step — when their label matches a planned item's
// label exactly (case-insensitive). Unlike SuggestMatches (bare amount+date,
// ambiguous when several recurring items share an amount), a label match is
// specific enough to apply on its own: the user already told the system
// "this exact label is this recurring item" by creating it (often straight
// from an operation with that label), so every operation with the same
// label — past or future — is that same recurring charge/income.
//
// It first tries to fill a pending forecast occurrence (same amount, within
// the usual date tolerance) so previsions stay accurate. Failing that — most
// often for operations older than the item's start date, which never got a
// forecast occurrence generated for them — it backfills a new occurrence
// dated at the operation's own date, straight in "reconciled" status, as
// long as the amount matches the item's expected amount. This is what lets
// creating a recurring item from one operation retroactively reconcile every
// past operation sharing its label, not just future ones.
func (uc *ReconcileUseCase) AutoMatchByLabel(ctx context.Context, accountID uuid.UUID) ([]entities.PlannedOccurrence, error) {
	unreconciledOps, err := uc.operations.ListUnreconciled(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(unreconciledOps) == 0 {
		return nil, nil
	}

	items, err := uc.plannedItems.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	itemsByLabel := make(map[string]entities.PlannedItem, len(items))
	for _, item := range items {
		if item.MatchMode != entities.PlannedItemMatchModeLabel || item.Label == nil {
			continue
		}
		itemsByLabel[normalizeLabel(*item.Label)] = item
	}
	if len(itemsByLabel) == 0 {
		return nil, nil
	}

	var reconciled []entities.PlannedOccurrence
	for i := range unreconciledOps {
		op := &unreconciledOps[i]
		item, ok := itemsByLabel[normalizeLabel(op.Label)]
		if !ok {
			continue
		}

		occurrences, err := uc.occurrences.ListForItem(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		var best *entities.PlannedOccurrence
		bestDelta := -1
		for j := range occurrences {
			occ := &occurrences[j]
			if occ.Status != entities.OccurrenceForecast {
				continue
			}
			if !occ.ExpectedAmount.Decimal.Equal(op.Amount.Decimal) {
				continue
			}
			delta := daysBetween(op.Date, occ.DueDate)
			if delta > defaultDateToleranceDays {
				continue
			}
			if best == nil || delta < bestDelta {
				best, bestDelta = occ, delta
			}
		}
		if best == nil {
			backfilled, err := uc.backfillPastOccurrence(ctx, item, op)
			if err != nil {
				return nil, err
			}
			if backfilled == nil {
				continue
			}
			reconciled = append(reconciled, *backfilled)
			continue
		}

		if err := uc.applyItemCategory(ctx, best.PlannedItemID, op); err != nil {
			return nil, err
		}
		best.Status = entities.OccurrenceReconciled
		best.ReconciledOperationID = &op.ID
		amount := op.Amount
		best.ReconciledAmount = &amount
		updated, err := uc.occurrences.Update(ctx, best)
		if err != nil {
			return nil, err
		}
		reconciled = append(reconciled, *updated)
	}

	return reconciled, nil
}

// backfillPastOccurrence handles an unreconciled operation whose label
// matches item but which has no matching forecast occurrence — typically
// because it predates the item's start date. If its amount matches the
// item's expected amount and no occurrence already exists on that exact day
// (planned_item_id, due_date is unique), it creates one straight in
// "reconciled" status dated at the operation's own date, so past operations
// get linked to the recurring item too instead of only future ones.
func (uc *ReconcileUseCase) backfillPastOccurrence(ctx context.Context, item entities.PlannedItem, op *entities.Operation) (*entities.PlannedOccurrence, error) {
	expected := itemSignedAmount(item)
	if !expected.Decimal.Equal(op.Amount.Decimal) {
		return nil, nil
	}
	existing, err := uc.occurrences.GetByItemAndDate(ctx, item.ID, op.Date)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, nil
	}
	if err := uc.applyItemCategory(ctx, item.ID, op); err != nil {
		return nil, err
	}
	amount := op.Amount
	created, err := uc.occurrences.Add(ctx, &entities.PlannedOccurrence{
		ID:                    uuid.New(),
		PlannedItemID:         item.ID,
		DueDate:               op.Date,
		ExpectedAmount:        expected,
		Status:                entities.OccurrenceReconciled,
		ReconciledOperationID: &op.ID,
		ReconciledAmount:      &amount,
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// CategoryBudgetActuals computes how much was actually spent/received in
// each of the given occurrences that belongs to a category-mode item — the
// sum of every operation in the account carrying that category, dated within
// the occurrence's own period. A period runs from the day after the item's
// previous occurrence (or the item's start date, for the first one) through
// the occurrence's own due date, so periods tile the item's whole history
// with no gaps or overlaps regardless of its recurrence cadence.
//
// Always computed fresh, never persisted: unlike a label-mode item's single
// reconciled_operation_id, there's no one operation to point at, and
// recomputing on every read means re-categorizing a past operation instantly
// corrects historical budget figures instead of leaving them stale.
// Occurrences belonging to a label-mode (or unknown) item are simply omitted
// from the result, not zeroed — callers should treat a missing entry as "not
// a category budget", not "spent nothing".
func (uc *ReconcileUseCase) CategoryBudgetActuals(ctx context.Context, accountID uuid.UUID, occurrences []entities.PlannedOccurrence) (map[uuid.UUID]money.Amount, error) {
	if len(occurrences) == 0 {
		return nil, nil
	}
	items, err := uc.plannedItems.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	itemsByID := make(map[uuid.UUID]entities.PlannedItem, len(items))
	for _, item := range items {
		itemsByID[item.ID] = item
	}

	requested := make(map[uuid.UUID]bool, len(occurrences))
	itemIDs := map[uuid.UUID]bool{}
	for _, occ := range occurrences {
		item, ok := itemsByID[occ.PlannedItemID]
		if !ok || item.MatchMode != entities.PlannedItemMatchModeCategory || item.CategoryID == nil {
			continue
		}
		requested[occ.ID] = true
		itemIDs[item.ID] = true
	}
	if len(itemIDs) == 0 {
		return nil, nil
	}

	result := make(map[uuid.UUID]money.Amount, len(requested))
	for itemID := range itemIDs {
		item := itemsByID[itemID]
		allOccs, err := uc.occurrences.ListForItem(ctx, itemID)
		if err != nil {
			return nil, err
		}
		if len(allOccs) == 0 {
			continue
		}
		sort.Slice(allOccs, func(i, j int) bool { return allOccs[i].DueDate.Before(allOccs[j].DueDate) })

		rangeStart := item.StartDate
		rangeEnd := allOccs[len(allOccs)-1].DueDate
		ops, err := uc.operations.ListForAccount(ctx, accountID, &rangeStart, &rangeEnd)
		if err != nil {
			return nil, err
		}

		periodStart := item.StartDate
		for _, occ := range allOccs {
			if requested[occ.ID] {
				sum := money.Zero()
				for _, op := range ops {
					if op.CategoryID == nil || *op.CategoryID != *item.CategoryID {
						continue
					}
					if op.Date.Before(periodStart) || op.Date.After(occ.DueDate) {
						continue
					}
					sum = sum.Add(op.Amount)
				}
				result[occ.ID] = sum
			}
			periodStart = occ.DueDate.AddDays(1)
		}
	}
	return result, nil
}
