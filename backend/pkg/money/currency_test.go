package money

import "testing"

func TestParseCurrency(t *testing.T) {
	for in, want := range map[string]Currency{"EUR": "EUR", "usd": "USD", " Gbp ": "GBP"} {
		got, err := ParseCurrency(in)
		if err != nil || got != want {
			t.Errorf("ParseCurrency(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "EU", "EUU", "euro", "KWD"} {
		if _, err := ParseCurrency(in); err == nil {
			t.Errorf("ParseCurrency(%q): expected an error, got nil", in)
		}
	}
}
