package services

import (
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/pkg/civil"
)

func TestComputeDedupHash_StableForIdenticalInputs(t *testing.T) {
	accountID := uuid.New()
	date := civil.New(2026, 1, 5)

	h1 := ComputeDedupHash(accountID, date, amt("-42.50"), "  Coffee   Shop ")
	h2 := ComputeDedupHash(accountID, date, amt("-42.50"), "coffee shop")

	if h1 != h2 {
		t.Errorf("expected whitespace/case-insensitive label normalization to produce the same hash, got %q vs %q", h1, h2)
	}
}

func TestComputeDedupHash_DiffersOnAmount(t *testing.T) {
	accountID := uuid.New()
	date := civil.New(2026, 1, 5)

	h1 := ComputeDedupHash(accountID, date, amt("-42.50"), "Coffee Shop")
	h2 := ComputeDedupHash(accountID, date, amt("-42.51"), "Coffee Shop")

	if h1 == h2 {
		t.Error("expected different amounts to produce different hashes")
	}
}
