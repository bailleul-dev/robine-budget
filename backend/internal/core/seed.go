package core

import (
	"context"
	"errors"
	"log"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
)

// SeedDevData creates a known-credentials account for local development,
// gated behind SEED_DEV_USER so it never runs unless explicitly opted into
// (e.g. never in a production deployment). Idempotent: reruns just find the
// email already there and no-op.
func SeedDevData(ctx context.Context, settings Settings, auth *services.AuthUseCase, users ports.UserRepository, clock ports.Clock) {
	if !settings.SeedDevUser {
		return
	}

	_, err := auth.Register(ctx, settings.SeedDevUserEmail, settings.SeedDevUserPassword, "Dev")
	if err != nil && !errors.Is(err, services.ErrEmailAlreadyRegistered) {
		log.Printf("seed dev user: %v", err)
		return
	}
	log.Printf("dev user ready: %s", settings.SeedDevUserEmail)
}

const creditMutuelProfileName = "Crédit Mutuel"

// SeedGlobalBankProfiles creates the built-in bank profiles shared with every
// user (UserID nil). Runs unconditionally on every startup — unlike
// SeedDevData it is not dev-only, since these profiles should be available in
// every environment. Idempotent: reruns just find the profile already there
// and no-op.
func SeedGlobalBankProfiles(ctx context.Context, bankProfiles ports.BankProfileRepository, clock ports.Clock) {
	if err := seedCreditMutuelProfile(ctx, bankProfiles, clock); err != nil {
		log.Printf("seed global bank profile: %v", err)
	}
}

// seedCreditMutuelProfile matches Crédit Mutuel's real CSV export: ";"
// delimited, Latin-1 encoded, with Débit/Crédit already signed (only one
// populated per row) rather than positive-magnitude columns.
func seedCreditMutuelProfile(ctx context.Context, bankProfiles ports.BankProfileRepository, clock ports.Clock) error {
	// Global profiles show up regardless of which user we list for, so any
	// user ID works here to fetch them.
	existing, err := bankProfiles.ListForUser(ctx, uuid.Nil)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.UserID == nil && p.Name == creditMutuelProfileName {
			return nil
		}
	}

	debit := "Débit"
	credit := "Crédit"
	now := clock.Now()
	_, err = bankProfiles.Add(ctx, &entities.BankProfile{
		ID:               uuid.New(),
		UserID:           nil,
		Name:             creditMutuelProfileName,
		Delimiter:        ";",
		Encoding:         "iso-8859-1",
		DateFormat:       "%d/%m/%Y",
		DecimalSeparator: ",",
		AmountMode:       entities.AmountModeDebitCreditSigned,
		ColumnMapping: entities.ColumnMapping{
			Date:   "Date",
			Label:  "Libellé",
			Debit:  &debit,
			Credit: &credit,
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return err
}
