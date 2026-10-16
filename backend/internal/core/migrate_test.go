package core

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTestDatabaseURL = "postgres://budget:budget@localhost:5432/budget_test?sslmode=disable"

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := defaultTestDatabaseURL
	if v, ok := os.LookupEnv("TEST_DATABASE_URL"); ok {
		url = v
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Skipf("skipping: no test database available (%v)", err)
	}
	pool.Close()
	return url
}

// The budget_test database (see internal/adapters/db's package doc for
// setup) already has every migration applied by the time any test runs -
// RunMigrations against it a second time must be a clean no-op, not an
// error, since that's exactly what happens on every real server restart.
func TestRunMigrations_AlreadyUpToDate(t *testing.T) {
	url := testDatabaseURL(t)
	if err := RunMigrations(url); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
}

func TestRunMigrations_InvalidDatabaseURL(t *testing.T) {
	testDatabaseURL(t) // still skip if there's no Postgres to test against at all
	err := RunMigrations("postgres://nobody:nothing@127.0.0.1:1/does-not-exist?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Fatal("expected an error connecting to an unreachable database, got nil")
	}
}
