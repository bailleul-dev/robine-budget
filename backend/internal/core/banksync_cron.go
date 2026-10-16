package core

import (
	"context"
	"log"
	"time"

	"bailleul.dev/budget-server/internal/domain/services"
)

// BankSyncCronHour is the UTC hour the daily bank sync cron fires at - early
// morning (well before anyone opens the app, outside the bank site's own
// peak hours). Deliberately not run once at startup too: `make dev` hot-
// reloads the server on every save (see air), and that must not trigger a
// real login against the bank on every reload.
const BankSyncCronHour = 3

// RunDailyBankSync runs useCase.SyncAll once a day at BankSyncCronHour UTC,
// until ctx is cancelled - the daily-history counterpart to the manual
// "Sync" button, so recurring-charge detection (which needs several months
// of operations for the same label to show up) actually gets there over
// time. Blocks forever; callers run it in its own goroutine.
func RunDailyBankSync(ctx context.Context, useCase *services.SyncAllBankConnectionsUseCase) {
	for {
		wait := time.Until(nextRunAt(time.Now().UTC(), BankSyncCronHour))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		log.Println("bank sync cron: starting daily sync...")
		results, err := useCase.SyncAll(ctx)
		if err != nil {
			log.Printf("bank sync cron: failed to list bank connections: %v", err)
			continue
		}
		for _, r := range results {
			if r.Err != nil {
				log.Printf("bank sync cron: user %s failed: %v", r.UserID, r.Err)
				continue
			}
			operationsAdded := 0
			for _, a := range r.Accounts {
				operationsAdded += a.OperationsAdded
			}
			log.Printf("bank sync cron: user %s synced (%d accounts, %d new operations)", r.UserID, len(r.Accounts), operationsAdded)
		}
	}
}

// nextRunAt returns the next UTC instant at the given hour (0-23, on the
// minute) strictly after now - today's if it hasn't passed yet, tomorrow's
// otherwise.
func nextRunAt(now time.Time, hour int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
