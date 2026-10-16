// Package money provides an exact decimal amount type shared across every
// layer (domain, db, http) — the one thing a budget app can never let drift
// to float64. Wraps shopspring/decimal so JSON encodes/decodes as a plain
// string ("123.45"), matching the wire shape the existing Python backend
// (and the Flutter/web clients built against it) already expect.
package money

import (
	"database/sql/driver"
	"fmt"

	"github.com/shopspring/decimal"
)

// Amount is a fixed-point decimal — never a float64 — used for every
// currency value (operation amounts, balances, planned item amounts).
type Amount struct {
	decimal.Decimal
}

func Zero() Amount { return Amount{decimal.Zero} }

// New parses an amount from outside input (an API body, a CSV cell, a bank
// feed). It rejects anything finer than a cent instead of letting Postgres
// silently round it on write (columns are NUMERIC(14, 2)): "10.005" is an
// error, while "10.050" is fine since it's exactly 10.05.
func New(value string) (Amount, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return Amount{}, fmt.Errorf("money: invalid amount %q: %w", value, err)
	}
	if !d.Equal(d.Round(2)) {
		return Amount{}, fmt.Errorf("money: invalid amount %q: more than 2 decimal places", value)
	}
	return Amount{d}, nil
}

func (a Amount) Add(other Amount) Amount { return Amount{a.Decimal.Add(other.Decimal)} }
func (a Amount) Sub(other Amount) Amount { return Amount{a.Decimal.Sub(other.Decimal)} }
func (a Amount) Neg() Amount             { return Amount{a.Decimal.Neg()} }
func (a Amount) IsZero() bool            { return a.Decimal.IsZero() }
func (a Amount) LessThan(other Amount) bool {
	return a.Decimal.LessThan(other.Decimal)
}

// MarshalJSON always renders as a quoted decimal string ("1234.56"), never a
// JSON number — the same convention FastAPI/Pydantic uses for Decimal
// fields, so existing clients need no changes.
func (a Amount) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.Decimal.StringFixed(2) + `"`), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	amount, err := New(s)
	if err != nil {
		return err
	}
	*a = amount
	return nil
}

// Value/Scan let Amount be used directly as a pgx/database-sql column type,
// stored as Postgres NUMERIC — mirrors SQLAlchemy's Numeric(14, 2) columns.
func (a Amount) Value() (driver.Value, error) {
	return a.Decimal.Value()
}

func (a *Amount) Scan(value any) error {
	var d decimal.Decimal
	if err := d.Scan(value); err != nil {
		return err
	}
	a.Decimal = d
	return nil
}
