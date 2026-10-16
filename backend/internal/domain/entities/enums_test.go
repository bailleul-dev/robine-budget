package entities

import (
	"errors"
	"testing"
)

func TestParseEnums_RoundTripKnownValues(t *testing.T) {
	for _, v := range []RecurrenceType{RecurrenceOneOff, RecurrencePerpetual, RecurrenceFixedCount, RecurrenceEveryXWeeks} {
		if got, err := ParseRecurrenceType(string(v)); err != nil || got != v {
			t.Errorf("ParseRecurrenceType(%q) = %q, %v", v, got, err)
		}
	}
	for _, v := range []PlannedItemKind{PlannedItemKindCharge, PlannedItemKindIncome} {
		if got, err := ParsePlannedItemKind(string(v)); err != nil || got != v {
			t.Errorf("ParsePlannedItemKind(%q) = %q, %v", v, got, err)
		}
	}
	for _, v := range []PlannedItemMatchMode{PlannedItemMatchModeLabel, PlannedItemMatchModeCategory} {
		if got, err := ParsePlannedItemMatchMode(string(v)); err != nil || got != v {
			t.Errorf("ParsePlannedItemMatchMode(%q) = %q, %v", v, got, err)
		}
	}
	for _, v := range []AmountMode{AmountModeSingle, AmountModeDebitCredit, AmountModeDebitCreditSigned} {
		if got, err := ParseAmountMode(string(v)); err != nil || got != v {
			t.Errorf("ParseAmountMode(%q) = %q, %v", v, got, err)
		}
	}
}

func TestParseEnums_RejectUnknownValues(t *testing.T) {
	errs := map[string]error{}
	_, errs["recurrence"] = ParseRecurrenceType("yearly")
	_, errs["kind"] = ParsePlannedItemKind("")
	_, errs["match mode"] = ParsePlannedItemMatchMode("LABEL")
	_, errs["amount mode"] = ParseAmountMode("signed")
	for name, err := range errs {
		if !errors.Is(err, ErrInvalidEnum) {
			t.Errorf("%s: err = %v, want ErrInvalidEnum", name, err)
		}
	}
}
