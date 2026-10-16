// Integration tests for this package talk to a real Postgres — the SQL
// itself (joins, cursor pagination, cascades) is exactly what a fake
// in-memory repo (see internal/domain/services' tests) can't catch. They
// target a dedicated "budget_test" database (never the "budget" dev
// database these tests could otherwise pollute), same server as
// docker-compose.yml, migrated once with:
//
//	docker compose exec -T postgres psql -U budget -d postgres -c "CREATE DATABASE budget_test OWNER budget;"
//	migrate -path migrations -database "postgres://budget:budget@localhost:5432/budget_test?sslmode=disable" up
//
// If that database isn't reachable (a fresh checkout that hasn't run the
// two commands above, or a CI environment with no Postgres at all), every
// test in this package skips rather than fails — go test ./... elsewhere
// in the module still runs and still means something.
package db

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

const defaultTestDatabaseURL = "postgres://budget:budget@localhost:5432/budget_test?sslmode=disable"

func testDatabaseURL() string {
	if v, ok := os.LookupEnv("TEST_DATABASE_URL"); ok {
		return v
	}
	return defaultTestDatabaseURL
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	pool, err := NewPool(ctx, testDatabaseURL())
	if err != nil {
		t.Skipf("skipping: no test database available (%v) — see this file's package doc for setup", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// createTestUser inserts a minimal user for a test to hang accounts/
// categories/etc. off of, and cleans it up afterward. Registered cleanups
// run LIFO (last registered first), so as long as every dependent row this
// test creates is created — and its own cleanup registered — after this
// call, everything referencing this user is already gone by the time this
// cleanup actually runs (accounts.user_id has no cascade, so deleting the
// user first would otherwise fail with a foreign key violation).
func createTestUser(t *testing.T, pool *pgxpool.Pool) *entities.User {
	t.Helper()
	ctx := context.Background()
	email := "test-" + uuid.New().String() + "@example.com"
	user := &entities.User{ID: uuid.New(), Email: &email, DisplayName: "Test User"}
	created, err := NewUserRepository(pool).Add(ctx, user)
	if err != nil {
		t.Fatalf("createTestUser: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", created.ID); err != nil {
			t.Errorf("cleanup: delete test user: %v", err)
		}
	})
	return created
}

func createTestAccount(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) *entities.Account {
	t.Helper()
	ctx := context.Background()
	account := &entities.Account{ID: uuid.New(), UserID: userID, Name: "Test Account", Currency: "EUR"}
	created, err := NewAccountRepository(pool).Add(ctx, account)
	if err != nil {
		t.Fatalf("createTestAccount: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM accounts WHERE id = $1", created.ID); err != nil {
			t.Errorf("cleanup: delete test account: %v", err)
		}
	})
	return created
}

// mustCreateOperation inserts a minimal operation for accountID with a
// fresh dedup hash (so parallel tests never collide on the same
// (account_id, dedup_hash) uniqueness constraint) and registers its own
// cleanup - safe to call even after the test already deletes the whole
// account (accounts cascade to operations), since a delete of an
// already-gone row is just a no-op, not an error.
func mustCreateOperation(t *testing.T, pool *pgxpool.Pool, accountID uuid.UUID, label, amount string) *entities.Operation {
	t.Helper()
	amt, err := money.New(amount)
	if err != nil {
		t.Fatalf("money.New(%q): %v", amount, err)
	}
	op := &entities.Operation{
		ID:        uuid.New(),
		AccountID: accountID,
		Date:      civil.Today(),
		Label:     label,
		Amount:    amt,
		DedupHash: "test-" + uuid.New().String(),
	}
	created, err := NewOperationRepository(pool).Add(context.Background(), op)
	if err != nil {
		t.Fatalf("mustCreateOperation: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM operations WHERE id = $1", created.ID)
	})
	return created
}

func newTestUUID() uuid.UUID { return uuid.New() }

func newTestUUIDString() string { return uuid.New().String() }

// mustExec runs a one-off statement (typically test cleanup) and fails the
// test immediately if it errors, instead of every call site repeating the
// same three-line if-err-t.Fatalf.
func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("mustExec(%q): %v", sql, err)
	}
}

// createTestBankProfile inserts a global (no owning user) CSV import profile
// for a test to attach import batches to.
func createTestBankProfile(t *testing.T, pool *pgxpool.Pool) *entities.BankProfile {
	t.Helper()
	ctx := context.Background()
	profile := &entities.BankProfile{
		ID:               uuid.New(),
		Name:             "Test Profile " + uuid.New().String(),
		Delimiter:        ";",
		Encoding:         "utf-8",
		DateFormat:       "02/01/2006",
		DecimalSeparator: ",",
		AmountMode:       entities.AmountModeSingle,
		ColumnMapping:    entities.ColumnMapping{Date: "Date", Label: "Libelle", Amount: strPtr("Montant")},
	}
	created, err := NewBankProfileRepository(pool).Add(ctx, profile)
	if err != nil {
		t.Fatalf("createTestBankProfile: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM bank_profiles WHERE id = $1", created.ID); err != nil {
			t.Errorf("cleanup: delete test bank profile: %v", err)
		}
	})
	return created
}

func strPtr(s string) *string { return &s }

// createTestCategory inserts a standard (global, no owning user) category —
// every test in this package that needs a category to attach to an
// operation uses one of these rather than a real seeded category, so tests
// don't depend on (or collide with) the app's actual seed data.
func createTestCategory(t *testing.T, pool *pgxpool.Pool) *entities.Category {
	t.Helper()
	ctx := context.Background()
	category := &entities.Category{ID: uuid.New(), Name: "Test Category " + uuid.New().String()}
	created, err := NewCategoryRepository(pool).Add(ctx, category)
	if err != nil {
		t.Fatalf("createTestCategory: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM categories WHERE id = $1", created.ID); err != nil {
			t.Errorf("cleanup: delete test category: %v", err)
		}
	})
	return created
}
