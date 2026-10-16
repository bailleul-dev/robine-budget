// Use-case: scan an account's imported operations for spending/income
// patterns that repeat on a regular schedule but aren't tracked by any
// PlannedItem yet, and surface them as suggestions the user can turn into a
// recurring item with one click (see CreateRecurringItemDialog on the
// frontend, which this feeds).
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

// minRecurringOccurrences: fewer than this and a repeating label is just as
// likely coincidence (two unrelated purchases from the same merchant) as an
// actual recurring charge.
const minRecurringOccurrences = 3

// weeklyToleranceDays/monthlyToleranceDays absorb the date jitter real bank
// processing introduces (weekends, holidays, varying month lengths) without
// letting genuinely irregular spending pass as "recurring".
const (
	weeklyToleranceDays  = 2
	monthlyToleranceDays = 4
	maxWeeklyInterval    = 4
)

// RecurringSuggestion is a detected candidate for a new PlannedItem, built
// from the most recent operation in the matching group (its label, amount,
// category and date anchor a create-item form the same way
// CreateRecurringItemDialog prefills one from a single operation).
type RecurringSuggestion struct {
	Label           string
	Kind            entities.PlannedItemKind
	SuggestedAmount money.Amount // always positive, matches PlannedItem.Amount's convention
	AnchorDate      civil.Date   // most recent matching operation's date
	RecurrenceType  entities.RecurrenceType
	IntervalWeeks   *int
	DayOfMonth      *int
	CategoryID      *uuid.UUID
	OccurrenceCount int
	LastOperationID uuid.UUID
}

type RecurringDetectionUseCase struct {
	operations   ports.OperationRepository
	plannedItems ports.PlannedItemRepository
}

func NewRecurringDetectionUseCase(
	operations ports.OperationRepository,
	plannedItems ports.PlannedItemRepository,
) *RecurringDetectionUseCase {
	return &RecurringDetectionUseCase{operations: operations, plannedItems: plannedItems}
}

// Detect groups an account's operations by normalized label, drops groups
// already covered by an existing PlannedItem (AutoMatchByLabel already
// reconciles those going forward), and returns one suggestion per remaining
// group whose dates fall on a regular weekly or monthly schedule. Never
// creates anything — purely advisory.
func (uc *RecurringDetectionUseCase) Detect(ctx context.Context, accountID uuid.UUID) ([]RecurringSuggestion, error) {
	ops, err := uc.operations.ListForAccount(ctx, accountID, nil, nil)
	if err != nil {
		return nil, err
	}

	items, err := uc.plannedItems.ListForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Label != nil {
			tracked[normalizeLabel(*item.Label)] = true
		}
	}

	groups := map[string][]entities.Operation{}
	for _, op := range ops {
		key := normalizeLabel(op.Label)
		if tracked[key] {
			continue
		}
		groups[key] = append(groups[key], op)
	}

	var suggestions []RecurringSuggestion
	for _, group := range groups {
		if suggestion, ok := detectRecurringGroup(group); ok {
			suggestions = append(suggestions, suggestion)
		}
	}
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].AnchorDate.After(suggestions[j].AnchorDate)
	})
	return suggestions, nil
}

// detectRecurringGroup decides whether one label's operations form a regular
// pattern. It keeps only the majority sign (a stray refund/reversal
// shouldn't sink an otherwise-regular charge) and requires the surviving
// dates to land on a consistent weekly or monthly cadence.
func detectRecurringGroup(ops []entities.Operation) (RecurringSuggestion, bool) {
	sort.Slice(ops, func(i, j int) bool { return ops[i].Date.Before(ops[j].Date) })

	negative := 0
	for _, op := range ops {
		if op.Amount.Decimal.IsNegative() {
			negative++
		}
	}
	wantNegative := negative*2 >= len(ops)
	var filtered []entities.Operation
	for _, op := range ops {
		if op.Amount.Decimal.IsNegative() == wantNegative {
			filtered = append(filtered, op)
		}
	}
	if len(filtered) < minRecurringOccurrences {
		return RecurringSuggestion{}, false
	}
	ops = filtered

	recurrenceType, intervalWeeks, dayOfMonth, ok := classifyRecurrence(ops)
	if !ok {
		return RecurringSuggestion{}, false
	}

	anchor := ops[len(ops)-1]
	kind := entities.PlannedItemKindCharge
	if !wantNegative {
		kind = entities.PlannedItemKindIncome
	}
	amount := anchor.Amount
	if amount.Decimal.IsNegative() {
		amount = amount.Neg()
	}

	return RecurringSuggestion{
		Label:           anchor.Label,
		Kind:            kind,
		SuggestedAmount: amount,
		AnchorDate:      anchor.Date,
		RecurrenceType:  recurrenceType,
		IntervalWeeks:   intervalWeeks,
		DayOfMonth:      dayOfMonth,
		CategoryID:      anchor.CategoryID,
		OccurrenceCount: len(ops),
		LastOperationID: anchor.ID,
	}, true
}

// classifyRecurrence tries a monthly cadence (same day of month, clamped for
// short months, like PlannedItem's own recurrence) before a weekly one,
// since most real recurring bills are monthly and a 28-30 day gap would
// otherwise also satisfy "every 4 weeks". ops must be sorted ascending by
// date and have at least minRecurringOccurrences entries.
func classifyRecurrence(ops []entities.Operation) (entities.RecurrenceType, *int, *int, bool) {
	allMonthly := true
	for i := 1; i < len(ops); i++ {
		expected := addMonthsClamped(ops[i-1].Date, 1, ops[i-1].Date.Time.Day())
		if daysBetween(expected, ops[i].Date) > monthlyToleranceDays {
			allMonthly = false
			break
		}
	}
	if allMonthly {
		dayOfMonth := ops[len(ops)-1].Date.Time.Day()
		return entities.RecurrencePerpetual, nil, &dayOfMonth, true
	}

	gaps := make([]int, 0, len(ops)-1)
	for i := 1; i < len(ops); i++ {
		gaps = append(gaps, daysBetween(ops[i-1].Date, ops[i].Date))
	}
	for weeks := 1; weeks <= maxWeeklyInterval; weeks++ {
		target := weeks * 7
		allMatch := true
		for _, gap := range gaps {
			if absInt(gap-target) > weeklyToleranceDays {
				allMatch = false
				break
			}
		}
		if allMatch {
			interval := weeks
			return entities.RecurrenceEveryXWeeks, &interval, nil, true
		}
	}

	return "", nil, nil, false
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
