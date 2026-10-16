// Use-case: materialize PlannedItem recurrence rules into PlannedOccurrence
// rows. Mirrors backend/app/domain/services/recurrence.py.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
)

const maxOccurrences = 1000 // safety cap against pathological windows/config

// addMonthsClamped adds months to base's month, keeping dayOfMonth but
// clamped to the shorter month's actual last day (e.g. day 31 in February).
func addMonthsClamped(base civil.Date, months int, dayOfMonth int) civil.Date {
	total := int(base.Time.Month()) - 1 + months
	year := base.Time.Year() + total/12
	month := total%12 + 1
	if month <= 0 {
		month += 12
		year--
	}
	firstOfMonth := civil.New(year, time.Month(month), 1)
	lastDay := firstOfMonth.LastDayOfMonth()
	day := dayOfMonth
	if day > lastDay {
		day = lastDay
	}
	return civil.New(year, time.Month(month), day)
}

// CandidateDueDates returns every occurrence date for item up to windowEnd
// (inclusive), same semantics as the Python candidate_due_dates function.
func CandidateDueDates(item entities.PlannedItem, windowEnd civil.Date) []civil.Date {
	switch item.RecurrenceType {
	case entities.RecurrenceOneOff:
		if !item.StartDate.After(windowEnd) {
			return []civil.Date{item.StartDate}
		}
		return nil

	case entities.RecurrenceEveryXWeeks:
		interval := 1
		if item.IntervalWeeks != nil {
			interval = *item.IntervalWeeks
		}
		var dates []civil.Date
		current := item.StartDate
		for !current.After(windowEnd) && len(dates) < maxOccurrences {
			dates = append(dates, current)
			current = current.AddDays(7 * interval)
		}
		return dates

	case entities.RecurrencePerpetual, entities.RecurrenceFixedCount: // monthly on day_of_month (clamped for short months)
		dayOfMonth := item.StartDate.Time.Day()
		if item.DayOfMonth != nil {
			dayOfMonth = *item.DayOfMonth
		}
		var dates []civil.Date
		for index := 0; index < maxOccurrences; index++ {
			if item.RecurrenceType == entities.RecurrenceFixedCount && item.OccurrenceCount != nil && index >= *item.OccurrenceCount {
				break
			}
			due := addMonthsClamped(item.StartDate, index, dayOfMonth)
			if due.After(windowEnd) {
				break
			}
			dates = append(dates, due)
		}
		return dates

	default:
		return nil
	}
}

type GenerateOccurrencesUseCase struct {
	plannedItems ports.PlannedItemRepository
	occurrences  ports.PlannedOccurrenceRepository
}

func NewGenerateOccurrencesUseCase(
	plannedItems ports.PlannedItemRepository,
	occurrences ports.PlannedOccurrenceRepository,
) *GenerateOccurrencesUseCase {
	return &GenerateOccurrencesUseCase{plannedItems: plannedItems, occurrences: occurrences}
}

func (uc *GenerateOccurrencesUseCase) Execute(
	ctx context.Context,
	plannedItemID uuid.UUID,
	windowStart, windowEnd civil.Date,
) ([]entities.PlannedOccurrence, error) {
	item, err := uc.plannedItems.GetByID(ctx, plannedItemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("planned item %s not found", plannedItemID)
	}
	if !item.Active {
		return nil, nil
	}

	signedAmount := itemSignedAmount(*item)

	var created []entities.PlannedOccurrence
	for _, dueDate := range CandidateDueDates(*item, windowEnd) {
		if dueDate.Before(windowStart) {
			continue
		}
		existing, err := uc.occurrences.GetByItemAndDate(ctx, item.ID, dueDate)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			continue
		}
		created = append(created, entities.PlannedOccurrence{
			ID:             uuid.New(),
			PlannedItemID:  item.ID,
			DueDate:        dueDate,
			ExpectedAmount: signedAmount,
			Status:         entities.OccurrenceForecast,
		})
	}

	if len(created) > 0 {
		return uc.occurrences.AddMany(ctx, created)
	}
	return created, nil
}
