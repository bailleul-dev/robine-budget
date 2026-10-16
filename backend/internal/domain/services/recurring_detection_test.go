package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func addOp(repo *fakeOperationRepo, accountID uuid.UUID, date civil.Date, label, amount string) {
	op := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: date, Label: label, Amount: amt(amount)}
	_, _ = repo.Add(context.Background(), op)
}

func TestRecurringDetection_MonthlyChargeDetected(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 2, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 3, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 4, 6), "Netflix", "-13.49") // one day of jitter

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Label != "Netflix" {
		t.Errorf("expected label Netflix, got %q", s.Label)
	}
	if s.Kind != entities.PlannedItemKindCharge {
		t.Errorf("expected kind charge, got %q", s.Kind)
	}
	if !s.SuggestedAmount.Decimal.Equal(amt("13.49").Decimal) {
		t.Errorf("expected suggested amount 13.49, got %s", s.SuggestedAmount.Decimal)
	}
	if s.RecurrenceType != entities.RecurrencePerpetual {
		t.Errorf("expected perpetual recurrence, got %q", s.RecurrenceType)
	}
	if s.DayOfMonth == nil || *s.DayOfMonth != 6 {
		t.Errorf("expected day_of_month 6 (from the most recent op), got %v", s.DayOfMonth)
	}
	if s.OccurrenceCount != 4 {
		t.Errorf("expected occurrence count 4, got %d", s.OccurrenceCount)
	}
}

func TestRecurringDetection_WeeklyIncomeDetected(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 2), "Freelance Client", "250.00")
	addOp(operations, accountID, civil.New(2026, 1, 16), "Freelance Client", "250.00")
	addOp(operations, accountID, civil.New(2026, 1, 30), "Freelance Client", "250.00")
	addOp(operations, accountID, civil.New(2026, 2, 13), "Freelance Client", "250.00")

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Kind != entities.PlannedItemKindIncome {
		t.Errorf("expected kind income, got %q", s.Kind)
	}
	if s.RecurrenceType != entities.RecurrenceEveryXWeeks {
		t.Errorf("expected every_x_weeks recurrence, got %q", s.RecurrenceType)
	}
	if s.IntervalWeeks == nil || *s.IntervalWeeks != 2 {
		t.Errorf("expected interval_weeks 2, got %v", s.IntervalWeeks)
	}
}

func TestRecurringDetection_TooFewOccurrencesIgnored(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 2, 5), "Netflix", "-13.49")

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions with only 2 occurrences, got %d", len(suggestions))
	}
}

func TestRecurringDetection_IrregularDatesIgnored(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 3), "Corner Shop", "-8.20")
	addOp(operations, accountID, civil.New(2026, 1, 11), "Corner Shop", "-12.40")
	addOp(operations, accountID, civil.New(2026, 1, 19), "Corner Shop", "-6.10")
	addOp(operations, accountID, civil.New(2026, 2, 2), "Corner Shop", "-9.90")

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected no suggestions for irregular gaps, got %d", len(suggestions))
	}
}

func TestRecurringDetection_AlreadyTrackedItemExcluded(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 2, 5), "Netflix", "-13.49")
	addOp(operations, accountID, civil.New(2026, 3, 5), "Netflix", "-13.49")

	_, _ = plannedItems.Add(context.Background(), &entities.PlannedItem{
		ID: uuid.New(), AccountID: accountID, Kind: entities.PlannedItemKindCharge,
		Label: strPtr("netflix"), Amount: amt("13.49"), StartDate: civil.New(2026, 1, 5),
		RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("expected already-tracked label to be excluded, got %d suggestions", len(suggestions))
	}
}

func TestRecurringDetection_StrayRefundDoesNotBlockDetection(t *testing.T) {
	operations := newFakeOperationRepo()
	plannedItems := newFakePlannedItemRepo()
	accountID := uuid.New()

	addOp(operations, accountID, civil.New(2026, 1, 5), "Gym Membership", "-29.99")
	addOp(operations, accountID, civil.New(2026, 2, 5), "Gym Membership", "-29.99")
	addOp(operations, accountID, civil.New(2026, 2, 20), "Gym Membership", "29.99") // one-off refund, opposite sign
	addOp(operations, accountID, civil.New(2026, 3, 5), "Gym Membership", "-29.99")

	uc := NewRecurringDetectionUseCase(operations, plannedItems)
	suggestions, err := uc.Detect(context.Background(), accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion despite the stray refund, got %d", len(suggestions))
	}
	if suggestions[0].OccurrenceCount != 3 {
		t.Errorf("expected the refund to be excluded from the count, got %d", suggestions[0].OccurrenceCount)
	}
}
