package core

import (
	"context"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/domain/services"
)

// A cancelled context makes RunDailyBankSync return immediately from its
// select, before ever waiting for the next 3am UTC run or calling SyncAll -
// exactly the shutdown path the server's own goroutine relies on.
func TestRunDailyBankSync_StopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		RunDailyBankSync(ctx, services.NewSyncAllBankConnectionsUseCase(nil, nil))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunDailyBankSync did not return promptly after its context was cancelled")
	}
}

func TestNextRunAt(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "before today's run",
			now:  time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "after today's run",
			now:  time.Date(2026, 9, 16, 4, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 17, 3, 0, 0, 0, time.UTC),
		},
		{
			name: "exactly at today's run",
			now:  time.Date(2026, 9, 16, 3, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 17, 3, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextRunAt(tc.now, BankSyncCronHour)
			if !got.Equal(tc.want) {
				t.Errorf("nextRunAt(%v, %d) = %v, want %v", tc.now, BankSyncCronHour, got, tc.want)
			}
		})
	}
}
