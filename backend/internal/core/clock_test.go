package core

import (
	"testing"
	"time"
)

func TestSystemClock(t *testing.T) {
	clock := SystemClock{}

	now := clock.Now()
	if now.Location() != time.UTC {
		t.Errorf("Now() location = %v, want UTC", now.Location())
	}
	if since := time.Since(now); since < 0 || since > time.Minute {
		t.Errorf("Now() = %v looks implausible relative to the real current time", now)
	}

	today := clock.Today()
	if today.String() != now.Format("2006-01-02") {
		t.Errorf("Today() = %s, want it to match Now()'s own date (%s)", today, now.Format("2006-01-02"))
	}
}
