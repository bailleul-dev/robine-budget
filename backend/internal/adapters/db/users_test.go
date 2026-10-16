package db

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestUserRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)

	got, err := NewUserRepository(pool).GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Email == nil || *got.Email != *user.Email {
		t.Errorf("Email = %v, want %v", got.Email, user.Email)
	}
	if got.DisplayName != user.DisplayName {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, user.DisplayName)
	}
}

func TestUserRepository_GetByID_NotFound(t *testing.T) {
	pool := newTestPool(t)
	got, err := NewUserRepository(pool).GetByID(context.Background(), createTestUser(t, pool).ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected the just-created user, got nil")
	}
}

func TestUserRepository_GetByEmail(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)

	got, err := NewUserRepository(pool).GetByEmail(ctx, *user.Email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got == nil || got.ID != user.ID {
		t.Fatalf("got %v, want user %s", got, user.ID)
	}

	miss, err := NewUserRepository(pool).GetByEmail(ctx, "nobody-"+user.ID.String()+"@example.com")
	if err != nil {
		t.Fatalf("GetByEmail (miss): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil for an unknown email", miss)
	}
}

func TestUserRepository_GetByProviderSub(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewUserRepository(pool)

	googleSub := "google-" + newTestUUIDString()
	robineSub := "robine-" + newTestUUIDString()
	appleSub := "apple-" + newTestUUIDString()
	email := "linked-" + newTestUUIDString() + "@example.com"
	created, err := repo.Add(ctx, &entities.User{
		ID: newTestUUID(), Email: &email, DisplayName: "Linked User",
		GoogleSub: &googleSub, RobineSub: &robineSub, AppleSub: &appleSub,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM users WHERE id = $1", created.ID) })

	byGoogle, err := repo.GetByGoogleSub(ctx, googleSub)
	if err != nil {
		t.Fatalf("GetByGoogleSub: %v", err)
	}
	if byGoogle == nil || byGoogle.ID != created.ID {
		t.Fatalf("GetByGoogleSub: got %v, want %s", byGoogle, created.ID)
	}

	byApple, err := repo.GetByAppleSub(ctx, appleSub)
	if err != nil {
		t.Fatalf("GetByAppleSub: %v", err)
	}
	if byApple == nil || byApple.ID != created.ID {
		t.Fatalf("GetByAppleSub: got %v, want %s", byApple, created.ID)
	}

	byRobine, err := repo.GetByRobineSub(ctx, robineSub)
	if err != nil {
		t.Fatalf("GetByRobineSub: %v", err)
	}
	if byRobine == nil || byRobine.ID != created.ID {
		t.Fatalf("GetByRobineSub: got %v, want %s", byRobine, created.ID)
	}

	// The two providers are independent columns - a Google sub must never
	// accidentally match a lookup by Robine sub or vice versa.
	if got, err := repo.GetByRobineSub(ctx, googleSub); err != nil || got != nil {
		t.Fatalf("GetByRobineSub(googleSub) = %v, %v, want nil, nil", got, err)
	}
}

func TestUserRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)

	user.DisplayName = "Updated Name"
	updated, err := NewUserRepository(pool).Update(ctx, user)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "Updated Name" {
		t.Fatalf("DisplayName = %q, want Updated Name", updated.DisplayName)
	}

	reloaded, err := NewUserRepository(pool).GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if reloaded.DisplayName != "Updated Name" {
		t.Fatalf("persisted DisplayName = %q, want Updated Name", reloaded.DisplayName)
	}
}

// Deleting a user must take everything they own with it, and must not
// break (or wrongly destroy) another user's data that happens to reference
// something of theirs through a shared account.
func TestUserRepository_Delete_CascadesAndSparesOthers(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	gone := createTestUser(t, pool)
	stays := createTestUser(t, pool)

	// Everything "gone" owns.
	goneAccount := createTestAccount(t, pool, gone.ID)
	goneOp := mustCreateOperation(t, pool, goneAccount.ID, "Loyer", "-650.00")
	goneCategory, goneSubcategory, goneProfile := uuid.New(), uuid.New(), uuid.New()
	mustExec(t, pool, `INSERT INTO categories (id, user_id, parent_id, name) VALUES ($1, $2, NULL, 'Mine')`, goneCategory, gone.ID)
	mustExec(t, pool, `INSERT INTO categories (id, user_id, parent_id, name) VALUES ($1, $2, $3, 'Mine too')`, goneSubcategory, gone.ID, goneCategory)
	mustExec(t, pool, `INSERT INTO bank_profiles (id, user_id, name, delimiter, encoding, date_format, decimal_separator, amount_mode, column_mapping)
		VALUES ($1, $2, 'Mine', ';', 'utf-8', '02/01/2006', ',', 'single', '{}')`, goneProfile, gone.ID)
	mustExec(t, pool, `INSERT INTO planned_items (id, account_id, kind, label, amount, start_date, recurrence_type, active, category_id, match_mode)
		VALUES ($1, $2, 'charge', NULL, 400, CURRENT_DATE, 'perpetual', true, $3, 'category')`, uuid.New(), goneAccount.ID, goneCategory)
	mustExec(t, pool, `INSERT INTO operation_category_splits (id, operation_id, category_id, amount) VALUES ($1, $2, $3, 650)`, uuid.New(), goneOp.ID, goneSubcategory)

	// "stays"' own data, some of it pointing at "gone"'s things (possible
	// through a shared account).
	staysAccount := createTestAccount(t, pool, stays.ID)
	staysOp := mustCreateOperation(t, pool, staysAccount.ID, "Courses", "-50.00")
	staysBatch, labelItem, envelopeItem := uuid.New(), uuid.New(), uuid.New()
	mustExec(t, pool, `INSERT INTO import_batches (id, account_id, bank_profile_id, filename, row_count, imported_count, duplicate_count, errors)
		VALUES ($1, $2, $3, 'x.csv', 1, 1, 0, '[]')`, staysBatch, staysAccount.ID, goneProfile)
	mustExec(t, pool, `INSERT INTO planned_items (id, account_id, kind, label, amount, start_date, recurrence_type, active, category_id, match_mode)
		VALUES ($1, $2, 'charge', 'Netflix', 13, CURRENT_DATE, 'perpetual', true, $3, 'label')`, labelItem, staysAccount.ID, goneCategory)
	mustExec(t, pool, `INSERT INTO planned_items (id, account_id, kind, label, amount, start_date, recurrence_type, active, category_id, match_mode)
		VALUES ($1, $2, 'charge', NULL, 200, CURRENT_DATE, 'perpetual', true, $3, 'category')`, envelopeItem, staysAccount.ID, goneSubcategory)
	mustExec(t, pool, `INSERT INTO comments (id, operation_id, user_id, body) VALUES ($1, $2, $3, 'hi')`, uuid.New(), staysOp.ID, gone.ID)
	mustExec(t, pool, `UPDATE operations SET category_id = $1 WHERE id = $2`, goneCategory, staysOp.ID)

	if err := NewUserRepository(pool).Delete(ctx, gone.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}
	for sql, want := range map[string]int{
		`SELECT count(*) FROM users WHERE id = $1`:              0,
		`SELECT count(*) FROM accounts WHERE user_id = $1`:      0,
		`SELECT count(*) FROM categories WHERE user_id = $1`:    0,
		`SELECT count(*) FROM bank_profiles WHERE user_id = $1`: 0,
		`SELECT count(*) FROM comments WHERE user_id = $1`:      0,
	} {
		if got := count(sql, gone.ID); got != want {
			t.Errorf("%s = %d, want %d", sql, got, want)
		}
	}
	if got := count(`SELECT count(*) FROM operations WHERE id = $1`, goneOp.ID); got != 0 {
		t.Errorf("gone user's operations should be deleted with their account")
	}

	// The other user's data survives, minus dangling references.
	if got := count(`SELECT count(*) FROM operations WHERE id = $1 AND category_id IS NULL`, staysOp.ID); got != 1 {
		t.Errorf("other user's operation should survive, uncategorized")
	}
	if got := count(`SELECT count(*) FROM import_batches WHERE id = $1 AND bank_profile_id IS NULL`, staysBatch); got != 1 {
		t.Errorf("other user's import batch should survive, without its bank profile")
	}
	if got := count(`SELECT count(*) FROM planned_items WHERE id = $1 AND category_id IS NULL`, labelItem); got != 1 {
		t.Errorf("other user's label-mode item should survive, uncategorized")
	}
	if got := count(`SELECT count(*) FROM planned_items WHERE id = $1`, envelopeItem); got != 0 {
		t.Errorf("other user's envelope on a deleted category can't exist anymore")
	}
}
