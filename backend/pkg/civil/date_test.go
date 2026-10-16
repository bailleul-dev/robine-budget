package civil

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	d := New(2026, time.March, 5)
	if d.String() != "2026-03-05" {
		t.Fatalf("got %s, want 2026-03-05", d.String())
	}
}

func TestParse(t *testing.T) {
	d, err := Parse("2026-03-05")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !d.Equal(New(2026, time.March, 5)) {
		t.Fatalf("got %s, want 2026-03-05", d)
	}

	if _, err := Parse("not-a-date"); err == nil {
		t.Fatal("expected an error for an invalid date")
	}
	if _, err := Parse("2026/03/05"); err == nil {
		t.Fatal("expected an error for a non-ISO separator")
	}
}

func TestFromTime(t *testing.T) {
	// A timestamp with a non-midnight time-of-day and a non-UTC offset
	// should still collapse to the right calendar date - the whole point
	// of a date-only type is dropping everything but year/month/day.
	loc := time.FixedZone("UTC+5", 5*60*60)
	ts := time.Date(2026, time.December, 31, 23, 30, 0, 0, loc)
	d := FromTime(ts)
	if d.String() != "2026-12-31" {
		t.Fatalf("got %s, want 2026-12-31", d)
	}
}

func TestAddDays(t *testing.T) {
	d := New(2026, time.February, 27)
	if got := d.AddDays(3); got.String() != "2026-03-02" {
		t.Fatalf("AddDays(3) = %s, want 2026-03-02 (crosses a month boundary)", got)
	}
	if got := d.AddDays(-30); got.String() != "2026-01-28" {
		t.Fatalf("AddDays(-30) = %s, want 2026-01-28", got)
	}
}

func TestAddMonths(t *testing.T) {
	// time.AddDate's own overflow behavior: Jan 31 + 1 month rolls into
	// March (Feb has no 31st) rather than clamping to Feb 28 - that
	// clamping is the caller's job (see addMonthsClamped in services),
	// this test documents AddMonths does NOT do it itself.
	d := New(2026, time.January, 31)
	if got := d.AddMonths(1); got.String() != "2026-03-03" {
		t.Fatalf("AddMonths(1) = %s, want 2026-03-03 (rolls over, unclamped)", got)
	}
}

func TestBeforeAfterEqual(t *testing.T) {
	a := New(2026, time.March, 5)
	b := New(2026, time.March, 6)

	if !a.Before(b) {
		t.Fatal("March 5 should be before March 6")
	}
	if a.After(b) {
		t.Fatal("March 5 should not be after March 6")
	}
	if !b.After(a) {
		t.Fatal("March 6 should be after March 5")
	}
	if !a.Equal(New(2026, time.March, 5)) {
		t.Fatal("two dates built from the same y/m/d should be equal")
	}
}

func TestLastDayOfMonth(t *testing.T) {
	cases := []struct {
		date Date
		want int
	}{
		{New(2026, time.February, 1), 28},  // 2026 is not a leap year
		{New(2028, time.February, 1), 29},  // 2028 is
		{New(2026, time.April, 15), 30},
		{New(2026, time.December, 31), 31}, // year rollover
	}
	for _, tc := range cases {
		if got := tc.date.LastDayOfMonth(); got != tc.want {
			t.Errorf("LastDayOfMonth(%s) = %d, want %d", tc.date, got, tc.want)
		}
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := New(2026, time.March, 5)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"2026-03-05"` {
		t.Fatalf("got %s, want \"2026-03-05\"", data)
	}

	var roundTripped Date
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !roundTripped.Equal(original) {
		t.Fatalf("round trip changed the value: %s != %s", roundTripped, original)
	}
}

func TestUnmarshalJSON_Invalid(t *testing.T) {
	var d Date
	if err := json.Unmarshal([]byte(`"nope"`), &d); err == nil {
		t.Fatal("expected an error unmarshaling an invalid date")
	}
}

func TestScan(t *testing.T) {
	var d Date
	ts := time.Date(2026, time.March, 5, 14, 30, 0, 0, time.UTC)
	if err := d.Scan(ts); err != nil {
		t.Fatalf("Scan(time.Time): %v", err)
	}
	if d.String() != "2026-03-05" {
		t.Fatalf("got %s, want 2026-03-05 (time-of-day dropped)", d)
	}

	var zero Date
	if err := zero.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) should not error: %v", err)
	}

	var bad Date
	if err := bad.Scan("2026-03-05"); err == nil {
		t.Fatal("expected an error scanning an unsupported type")
	}
}

func TestValue(t *testing.T) {
	d := New(2026, time.March, 5)
	v, err := d.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	ts, ok := v.(time.Time)
	if !ok {
		t.Fatalf("Value() returned %T, want time.Time", v)
	}
	if !ts.Equal(d.Time) {
		t.Fatalf("Value() = %v, want %v", ts, d.Time)
	}
}
