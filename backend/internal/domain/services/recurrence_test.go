package services

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

func intPtr(n int) *int { return &n }

func TestCandidateDueDates_Perpetual_ClampsShortMonths(t *testing.T) {
	item := entities.PlannedItem{
		StartDate:      civil.New(2026, 1, 31),
		RecurrenceType: entities.RecurrencePerpetual,
		DayOfMonth:     intPtr(31),
	}
	dates := CandidateDueDates(item, civil.New(2026, 4, 30))

	want := []civil.Date{
		civil.New(2026, 1, 31),
		civil.New(2026, 2, 28), // clamped: February has no 31st
		civil.New(2026, 3, 31),
		civil.New(2026, 4, 30), // clamped: April has no 31st
	}
	if len(dates) != len(want) {
		t.Fatalf("got %d dates, want %d: %v", len(dates), len(want), dates)
	}
	for i, d := range dates {
		if !d.Equal(want[i]) {
			t.Errorf("date %d = %s, want %s", i, d, want[i])
		}
	}
}

func TestCandidateDueDates_FixedCount_StopsAtOccurrenceCount(t *testing.T) {
	item := entities.PlannedItem{
		StartDate:       civil.New(2026, 1, 15),
		RecurrenceType:  entities.RecurrenceFixedCount,
		DayOfMonth:      intPtr(15),
		OccurrenceCount: intPtr(3),
	}
	dates := CandidateDueDates(item, civil.New(2027, 1, 1))
	if len(dates) != 3 {
		t.Fatalf("got %d dates, want 3: %v", len(dates), dates)
	}
}

func TestCandidateDueDates_EveryXWeeks(t *testing.T) {
	item := entities.PlannedItem{
		StartDate:      civil.New(2026, 1, 5),
		RecurrenceType: entities.RecurrenceEveryXWeeks,
		IntervalWeeks:  intPtr(2),
	}
	dates := CandidateDueDates(item, civil.New(2026, 2, 1))
	want := []civil.Date{
		civil.New(2026, 1, 5),
		civil.New(2026, 1, 19),
	}
	if len(dates) != len(want) {
		t.Fatalf("got %d dates, want %d: %v", len(dates), len(want), dates)
	}
	for i, d := range dates {
		if !d.Equal(want[i]) {
			t.Errorf("date %d = %s, want %s", i, d, want[i])
		}
	}
}

func TestCandidateDueDates_OneOff_OnlyWithinWindow(t *testing.T) {
	item := entities.PlannedItem{
		StartDate:      civil.New(2026, 3, 1),
		RecurrenceType: entities.RecurrenceOneOff,
	}
	if dates := CandidateDueDates(item, civil.New(2026, 2, 1)); len(dates) != 0 {
		t.Errorf("expected no dates before the start date, got %v", dates)
	}
	if dates := CandidateDueDates(item, civil.New(2026, 3, 1)); len(dates) != 1 {
		t.Errorf("expected exactly one date at the window boundary, got %v", dates)
	}
}

func TestGenerateOccurrencesUseCase_SkipsAlreadyMaterializedDates(t *testing.T) {
	items := newFakePlannedItemRepo()
	occurrences := newFakeOccurrenceRepo()
	uc := NewGenerateOccurrencesUseCase(items, occurrences)

	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: uuid.New(), Kind: entities.PlannedItemKindCharge,
		Amount: amt("800"), StartDate: civil.New(2026, 1, 1),
		RecurrenceType: entities.RecurrencePerpetual, DayOfMonth: intPtr(1), Active: true,
	}
	items.byID[item.ID] = item
	// Pre-existing occurrence for January — must not be duplicated.
	occurrences.byID[uuid.New()] = &entities.PlannedOccurrence{
		ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.New(2026, 1, 1),
		ExpectedAmount: amt("-800"), Status: entities.OccurrenceForecast,
	}

	created, err := uc.Execute(context.Background(), item.ID, civil.New(2026, 1, 1), civil.New(2026, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 { // Feb + Mar; Jan already existed
		t.Fatalf("got %d created occurrences, want 2: %+v", len(created), created)
	}
	for _, occ := range created {
		if !occ.ExpectedAmount.Decimal.Equal(amt("-800").Decimal) {
			t.Errorf("charge amount should be negated, got %s", occ.ExpectedAmount.Decimal)
		}
	}
}
