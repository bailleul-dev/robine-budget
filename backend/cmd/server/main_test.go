package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/adapters/db"
	"bailleul.dev/budget-server/internal/core"
)

const defaultTestDatabaseURL = "postgres://budget:budget@localhost:5432/budget_test?sslmode=disable"

// testSettings returns Settings pointed at the same dedicated budget_test
// database internal/adapters/db's own integration tests use (see that
// package's doc comment for the one-time setup), skipping the test entirely
// when it isn't reachable rather than failing - this composition root
// genuinely needs a real Postgres to wire up.
func testSettings(t *testing.T) core.Settings {
	t.Helper()
	url := defaultTestDatabaseURL
	if v, ok := os.LookupEnv("TEST_DATABASE_URL"); ok {
		url = v
	}
	pool, err := db.NewPool(context.Background(), url)
	if err != nil {
		t.Skipf("skipping: no test database available (%v)", err)
	}
	pool.Close()

	return core.Settings{
		Port:                  "0", // let the OS pick a free port
		DatabaseURL:           url,
		JWTSecret:             "test-secret",
		JWTExpireMinutes:      60,
		CORSOrigin:            "http://localhost:5173",
		BankSyncPythonBin:     "python3",
		BankSyncProjectDir:    ".",
		BankSyncEncryptionKey: strings.Repeat("0", 64),
	}
}

// run is the entire composition root (every repository, use-case and
// handler constructor, the router, and the HTTP server) - this test doesn't
// assert on any one wiring decision, only that the whole thing starts
// cleanly against a real database and shuts down on context cancellation,
// which is exactly what happens on every real deployment's rolling restart.
func TestRun_StartsAndShutsDownCleanly(t *testing.T) {
	settings := testSettings(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- run(ctx, settings) }()

	// Give the server goroutine a moment to actually start listening before
	// asking it to shut down again.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not shut down within 5s of its context being cancelled")
	}
}

func TestRun_InvalidEncryptionKeyFailsFast(t *testing.T) {
	settings := testSettings(t)
	settings.BankSyncEncryptionKey = "not-valid-hex"

	err := run(context.Background(), settings)
	if err == nil {
		t.Fatal("expected an error for an invalid BANK_SYNC_ENCRYPTION_KEY, got nil")
	}
}

func TestRun_UnreachableDatabaseFailsFast(t *testing.T) {
	settings := testSettings(t)
	settings.DatabaseURL = "postgres://nobody:nothing@127.0.0.1:1/does-not-exist?sslmode=disable&connect_timeout=1"

	err := run(context.Background(), settings)
	if err == nil {
		t.Fatal("expected an error connecting to an unreachable database, got nil")
	}
}

func TestNewOTAUpdatesHandler(t *testing.T) {
	if h, err := newOTAUpdatesHandler(core.Settings{}); err != nil || h != nil {
		t.Fatalf("unconfigured: got %v, %v; want nil, nil", h, err)
	}
	if h, err := newOTAUpdatesHandler(core.Settings{OTAUpdatesDir: t.TempDir()}); err != nil || h == nil {
		t.Fatalf("unsigned: got %v, %v; want a handler", h, err)
	}
	_, err := newOTAUpdatesHandler(core.Settings{OTAUpdatesDir: t.TempDir(), OTACodeSigningKeyPath: "/does/not/exist.pem"})
	if err == nil {
		t.Fatal("a configured but unreadable signing key must fail startup")
	}

	s3 := core.Settings{
		OTAS3Endpoint:        "http://garage.garage.svc:3900",
		OTAS3Region:          "garage",
		OTAS3Bucket:          "budget-ota",
		OTAS3AccessKeyID:     "GK0123",
		OTAS3SecretAccessKey: "secret",
	}
	if h, err := newOTAUpdatesHandler(s3); err != nil || h == nil {
		t.Fatalf("s3: got %v, %v; want a handler", h, err)
	}
	incomplete := s3
	incomplete.OTAS3SecretAccessKey = ""
	if _, err := newOTAUpdatesHandler(incomplete); err == nil {
		t.Fatal("an incomplete S3 configuration must fail startup")
	}
	both := s3
	both.OTAUpdatesDir = t.TempDir()
	if _, err := newOTAUpdatesHandler(both); err == nil {
		t.Fatal("configuring both a directory and a bucket must fail startup")
	}
}
