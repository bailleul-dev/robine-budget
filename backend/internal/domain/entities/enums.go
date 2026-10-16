package entities

import (
	"errors"
	"fmt"
)

// ErrInvalidEnum is wrapped by every Parse* function below, so an adapter
// can tell "the caller sent a value this enum doesn't have" (a 400) apart
// from anything else.
//
// Adapters turn outside strings into these enum types only through Parse*,
// never with a bare conversion like RecurrenceType(s): a conversion accepts
// any string, and the bad value then only surfaces as a Postgres enum error
// (a 500) or, worse, falls into some switch's default branch. Each Parse* is
// itself a switch, so the exhaustive linter flags it when a value is added.
var ErrInvalidEnum = errors.New("invalid value")

func invalidEnum(name, value string) error {
	return fmt.Errorf("%w for %s: %q", ErrInvalidEnum, name, value)
}

func ParsePlannedItemKind(s string) (PlannedItemKind, error) {
	switch v := PlannedItemKind(s); v {
	case PlannedItemKindCharge, PlannedItemKindIncome:
		return v, nil
	default:
		return "", invalidEnum("planned item kind", s)
	}
}

func ParseRecurrenceType(s string) (RecurrenceType, error) {
	switch v := RecurrenceType(s); v {
	case RecurrenceOneOff, RecurrencePerpetual, RecurrenceFixedCount, RecurrenceEveryXWeeks:
		return v, nil
	default:
		return "", invalidEnum("recurrence type", s)
	}
}

func ParsePlannedItemMatchMode(s string) (PlannedItemMatchMode, error) {
	switch v := PlannedItemMatchMode(s); v {
	case PlannedItemMatchModeLabel, PlannedItemMatchModeCategory:
		return v, nil
	default:
		return "", invalidEnum("match mode", s)
	}
}

func ParseAmountMode(s string) (AmountMode, error) {
	switch v := AmountMode(s); v {
	case AmountModeSingle, AmountModeDebitCredit, AmountModeDebitCreditSigned:
		return v, nil
	default:
		return "", invalidEnum("amount mode", s)
	}
}
