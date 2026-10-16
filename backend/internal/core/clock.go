package core

import (
	"time"

	"bailleul.dev/budget-server/pkg/civil"
)

// SystemClock is the default ports.Clock implementation backed by the real
// system time — mirrors backend/app/core/clock.py's SystemClock.
type SystemClock struct{}

func (SystemClock) Today() civil.Date { return civil.Today() }
func (SystemClock) Now() time.Time    { return time.Now().UTC() }
