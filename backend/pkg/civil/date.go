// Package civil provides a date-only type (no time-of-day, no timezone) —
// shared by the domain (due dates, statement dates), the db layer (Postgres
// DATE columns) and the http layer (JSON as "YYYY-MM-DD"), matching Python's
// datetime.date semantics used throughout the existing backend.
package civil

import (
	"database/sql/driver"
	"fmt"
	"time"
)

const layout = "2006-01-02"

type Date struct {
	time.Time
}

func New(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

func Today() Date {
	now := time.Now().UTC()
	return New(now.Year(), now.Month(), now.Day())
}

func Parse(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, fmt.Errorf("civil: invalid date %q: %w", s, err)
	}
	return Date{t}, nil
}

func FromTime(t time.Time) Date {
	return New(t.Year(), t.Month(), t.Day())
}

func (d Date) AddDays(days int) Date { return Date{d.Time.AddDate(0, 0, days)} }
func (d Date) AddMonths(months int) Date {
	return Date{d.Time.AddDate(0, months, 0)}
}
func (d Date) Before(other Date) bool { return d.Time.Before(other.Time) }
func (d Date) After(other Date) bool  { return d.Time.After(other.Time) }
func (d Date) Equal(other Date) bool  { return d.Time.Equal(other.Time) }
func (d Date) String() string         { return d.Time.Format(layout) }

// LastDayOfMonth returns the number of days in this date's month/year — used
// to clamp a recurring item's day-of-month for short months (e.g. day 31 in
// February), mirroring the Python backend's calendar.monthrange usage.
func (d Date) LastDayOfMonth() int {
	firstOfNext := time.Date(d.Time.Year(), d.Time.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return firstOfNext.AddDate(0, 0, -1).Day()
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Date) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Date) Value() (driver.Value, error) {
	return d.Time, nil
}

func (d *Date) Scan(value any) error {
	switch v := value.(type) {
	case time.Time:
		*d = FromTime(v)
		return nil
	case nil:
		return nil
	default:
		return fmt.Errorf("civil: cannot scan %T into Date", value)
	}
}
