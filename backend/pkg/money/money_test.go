package money

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestNew(t *testing.T) {
	amt, err := New("123.45")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if amt.Decimal.StringFixed(2) != "123.45" {
		t.Fatalf("got %s, want 123.45", amt.Decimal.StringFixed(2))
	}

	if _, err := New("not-a-number"); err == nil {
		t.Fatal("expected an error for an invalid amount, got nil")
	}
}

func TestZero(t *testing.T) {
	if !Zero().IsZero() {
		t.Fatal("Zero() should be zero")
	}
}

func TestNew_RejectsSubCentPrecision(t *testing.T) {
	for _, s := range []string{"10.005", "-0.001", "1.999"} {
		if _, err := New(s); err == nil {
			t.Errorf("New(%q): expected an error, got nil", s)
		}
	}
	// Trailing zeros past the cent are still an exact cent amount.
	amt, err := New("10.050")
	if err != nil {
		t.Fatalf("New(10.050): %v", err)
	}
	if amt.Decimal.StringFixed(2) != "10.05" {
		t.Fatalf("got %s, want 10.05", amt.Decimal.StringFixed(2))
	}
}

func TestArithmetic(t *testing.T) {
	a := mustAmount(t, "10.50")
	b := mustAmount(t, "3.25")

	if got := a.Add(b).Decimal.StringFixed(2); got != "13.75" {
		t.Fatalf("Add: got %s, want 13.75", got)
	}
	if got := a.Sub(b).Decimal.StringFixed(2); got != "7.25" {
		t.Fatalf("Sub: got %s, want 7.25", got)
	}
	if got := a.Neg().Decimal.StringFixed(2); got != "-10.50" {
		t.Fatalf("Neg: got %s, want -10.50", got)
	}
	if !b.LessThan(a) {
		t.Fatal("3.25 should be less than 10.50")
	}
	if a.LessThan(b) {
		t.Fatal("10.50 should not be less than 3.25")
	}
}

func TestIsZero(t *testing.T) {
	if !mustAmount(t, "0.00").IsZero() {
		t.Fatal("0.00 should be zero")
	}
	if !mustAmount(t, "0").IsZero() {
		t.Fatal("0 should be zero")
	}
	if mustAmount(t, "0.01").IsZero() {
		t.Fatal("0.01 should not be zero")
	}
}

// A negative amount that rounds all the way to zero at 2dp (e.g. -0.001)
// renders as the plain "0.00", not "-0.00" — shopspring/decimal drops the
// sign once StringFixed rounds a value to exactly zero. Documenting this
// as the actual, intentional behavior (an amount too small to show at 2dp
// precision is indistinguishable from zero either way) rather than
// asserting a sign that was never really there.
func TestMarshalJSON_RoundsToExactZeroDropsSign(t *testing.T) {
	amt := Amount{decimal.RequireFromString("-0.001")} // sub-cent: only reachable through arithmetic, not New
	data, err := json.Marshal(amt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"0.00"` {
		t.Fatalf("got %s, want \"0.00\"", data)
	}
}

// A negative amount that still rounds to a non-zero value at 2dp keeps its
// sign — this is the case that actually matters for a real charge.
func TestMarshalJSON_NegativeSignPreserved(t *testing.T) {
	amt := Amount{decimal.RequireFromString("-0.006")}
	data, err := json.Marshal(amt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"-0.01"` {
		t.Fatalf("got %s, want \"-0.01\"", data)
	}
}

func TestMarshalJSON_AlwaysAQuotedString(t *testing.T) {
	amt := mustAmount(t, "1234.5")
	data, err := json.Marshal(amt)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"1234.50"` {
		t.Fatalf("got %s, want \"1234.50\" (fixed 2dp, quoted)", data)
	}
}

func TestUnmarshalJSON(t *testing.T) {
	var amt Amount
	if err := json.Unmarshal([]byte(`"42.10"`), &amt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if amt.Decimal.StringFixed(2) != "42.10" {
		t.Fatalf("got %s, want 42.10", amt.Decimal.StringFixed(2))
	}

	var bad Amount
	if err := json.Unmarshal([]byte(`"nope"`), &bad); err == nil {
		t.Fatal("expected an error unmarshaling an invalid amount")
	}
	if err := json.Unmarshal([]byte(`"1.005"`), &bad); err == nil {
		t.Fatal("expected an error unmarshaling a sub-cent amount")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := mustAmount(t, "-56.78")
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTripped Amount
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !original.Decimal.Equal(roundTripped.Decimal) {
		t.Fatalf("round trip changed the value: %s != %s", original.Decimal, roundTripped.Decimal)
	}
}

func TestValueAndScan(t *testing.T) {
	original := mustAmount(t, "99.99")
	driverValue, err := original.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}

	var scanned Amount
	if err := scanned.Scan(driverValue); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !original.Decimal.Equal(scanned.Decimal) {
		t.Fatalf("Scan(Value()) changed the value: %s != %s", original.Decimal, scanned.Decimal)
	}
}

func mustAmount(t *testing.T, s string) Amount {
	t.Helper()
	amt, err := New(s)
	if err != nil {
		t.Fatalf("New(%q): %v", s, err)
	}
	return amt
}
