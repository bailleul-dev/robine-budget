package money

import (
	"fmt"
	"strings"
)

// Currency is an ISO 4217 code (e.g. "EUR"). Every Amount belongs to an
// account, and the account's Currency says what unit it's in — an Amount
// itself stays a bare number, since nothing ever combines two accounts'
// amounts server-side.
type Currency string

const DefaultCurrency Currency = "EUR"

// supportedCurrencies is deliberately a closed list rather than "any three
// letters": amounts are stored as NUMERIC(14, 2) and New rejects sub-cent
// precision, so a currency with 3 minor units (KWD, BHD...) can't be
// represented — and a typo like "EUU" shouldn't create an account. Add a
// code here (with at most 2 minor units) to support it.
var supportedCurrencies = map[Currency]bool{
	"EUR": true, "USD": true, "GBP": true, "CHF": true, "CAD": true,
	"AUD": true, "JPY": true, "SEK": true, "NOK": true, "DKK": true,
	"PLN": true, "CZK": true, "HUF": true, "RON": true, "MAD": true,
	"XOF": true, "XAF": true,
}

// ParseCurrency accepts a code in any case, surrounded by any whitespace
// ("eur", " EUR ").
func ParseCurrency(s string) (Currency, error) {
	c := Currency(strings.ToUpper(strings.TrimSpace(s)))
	if !supportedCurrencies[c] {
		return "", fmt.Errorf("money: unsupported currency %q", s)
	}
	return c, nil
}
