package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

var whitespaceRe = regexp.MustCompile(`\s+`)

func NormalizeLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(whitespaceRe.ReplaceAllString(label, " ")))
}

// ComputeDedupHash mirrors backend/app/domain/services/dedup.py's field
// order and separator. The amount is rendered as a fixed 2-decimal string
// rather than Python's Decimal.normalize() (which drops trailing zeros into
// scientific notation, e.g. "1E+2") — that quirk only matters for byte-exact
// parity with hashes the Python backend already wrote, not for correctness:
// this hash only has to be stable against future imports the Go backend
// itself computes.
func ComputeDedupHash(accountID uuid.UUID, date civil.Date, amount money.Amount, label string) string {
	normalized := NormalizeLabel(label)
	payload := fmt.Sprintf("%s|%s|%s|%s", accountID.String(), date.String(), amount.Decimal.StringFixed(2), normalized)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
