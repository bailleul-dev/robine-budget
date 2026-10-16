package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

func mustOpAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

func TestOperationRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	op := mustCreateOperation(t, pool, account.ID, "Test Label", "-42.50")

	got, err := NewOperationRepository(pool).GetByID(ctx, op.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Label != "Test Label" {
		t.Errorf("Label = %q, want Test Label", got.Label)
	}
	if !got.Amount.Decimal.Equal(mustOpAmount(t, "-42.50").Decimal) {
		t.Errorf("Amount = %s, want -42.50", got.Amount.Decimal)
	}
}

func TestOperationRepository_FindByReference(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	ref := "ref-" + newTestUUIDString()

	created, err := NewOperationRepository(pool).Add(ctx, &entities.Operation{
		ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Referenced",
		Amount: mustOpAmount(t, "-10.00"), RawReference: &ref, DedupHash: "test-" + uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operations WHERE id = $1", created.ID) })

	got, err := NewOperationRepository(pool).FindByReference(ctx, account.ID, ref)
	if err != nil {
		t.Fatalf("FindByReference: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("got %v, want %s", got, created.ID)
	}

	// Same reference, different account - references are scoped per
	// account, so this must not match.
	otherAccount := createTestAccount(t, pool, user.ID)
	miss, err := NewOperationRepository(pool).FindByReference(ctx, otherAccount.ID, ref)
	if err != nil {
		t.Fatalf("FindByReference (other account): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil (reference belongs to a different account)", miss)
	}
}

func TestOperationRepository_FindByDedupHash(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	op := mustCreateOperation(t, pool, account.ID, "Dedup Test", "-5.00")

	got, err := NewOperationRepository(pool).FindByDedupHash(ctx, account.ID, op.DedupHash)
	if err != nil {
		t.Fatalf("FindByDedupHash: %v", err)
	}
	if got == nil || got.ID != op.ID {
		t.Fatalf("got %v, want %s", got, op.ID)
	}

	miss, err := NewOperationRepository(pool).FindByDedupHash(ctx, account.ID, "no-such-hash")
	if err != nil {
		t.Fatalf("FindByDedupHash (miss): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil", miss)
	}
}

func TestOperationRepository_AddMany(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	ops := []entities.Operation{
		{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "First", Amount: mustOpAmount(t, "-1.00"), DedupHash: "test-" + uuid.New().String()},
		{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Second", Amount: mustOpAmount(t, "-2.00"), DedupHash: "test-" + uuid.New().String()},
	}
	created, err := NewOperationRepository(pool).AddMany(ctx, ops)
	if err != nil {
		t.Fatalf("AddMany: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("got %d created, want 2", len(created))
	}
	for _, op := range created {
		id := op.ID
		t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operations WHERE id = $1", id) })
	}
}

func TestOperationRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	category := createTestCategory(t, pool)
	op := mustCreateOperation(t, pool, account.ID, "Original Label", "-10.00")

	op.Label = "Updated Label"
	op.CategoryID = &category.ID
	updated, err := NewOperationRepository(pool).Update(ctx, op)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Label != "Updated Label" {
		t.Fatalf("Label = %q, want Updated Label", updated.Label)
	}
	if updated.CategoryID == nil || *updated.CategoryID != category.ID {
		t.Fatalf("CategoryID = %v, want %s", updated.CategoryID, category.ID)
	}
}

func TestOperationRepository_ListForAccount_DateFilter(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	repo := NewOperationRepository(pool)

	jan := civil.New(2026, time.January, 15)
	feb := civil.New(2026, time.February, 15)
	mar := civil.New(2026, time.March, 15)
	for _, d := range []civil.Date{jan, feb, mar} {
		created, err := repo.Add(ctx, &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: d, Label: "op", Amount: mustOpAmount(t, "-1.00"), DedupHash: "test-" + uuid.New().String()})
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		id := created.ID
		t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operations WHERE id = $1", id) })
	}

	all, err := repo.ListForAccount(ctx, account.ID, nil, nil)
	if err != nil {
		t.Fatalf("ListForAccount (unbounded): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d operations, want 3", len(all))
	}

	febOnwards, err := repo.ListForAccount(ctx, account.ID, &feb, nil)
	if err != nil {
		t.Fatalf("ListForAccount (from feb): %v", err)
	}
	if len(febOnwards) != 2 {
		t.Fatalf("got %d operations from Feb onwards, want 2", len(febOnwards))
	}

	janToFeb, err := repo.ListForAccount(ctx, account.ID, &jan, &feb)
	if err != nil {
		t.Fatalf("ListForAccount (jan-feb): %v", err)
	}
	if len(janToFeb) != 2 {
		t.Fatalf("got %d operations in [jan, feb], want 2", len(janToFeb))
	}

	// Newest first.
	if !all[0].Date.Equal(mar) {
		t.Errorf("all[0].Date = %s, want %s (newest first)", all[0].Date, mar)
	}
}

// The keyset pagination itself: fetching page by page with the previous
// page's cursor must return every operation exactly once, in the same
// newest-first order ListForAccount (unpaginated) would.
func TestOperationRepository_ListPageForAccount_Pagination(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	repo := NewOperationRepository(pool)

	const total = 7
	for i := 0; i < total; i++ {
		d := civil.New(2026, time.January, 1).AddDays(i) // distinct dates, unambiguous DESC order
		created, err := repo.Add(ctx, &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: d, Label: "op", Amount: mustOpAmount(t, "-1.00"), DedupHash: "test-" + uuid.New().String()})
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		id := created.ID
		t.Cleanup(func() { mustExec(t, pool, "DELETE FROM operations WHERE id = $1", id) })
	}

	want, err := repo.ListForAccount(ctx, account.ID, nil, nil)
	if err != nil {
		t.Fatalf("ListForAccount: %v", err)
	}
	if len(want) != total {
		t.Fatalf("got %d operations, want %d", len(want), total)
	}

	const pageSize = 3
	var got []entities.Operation
	var after *entities.OperationPageCursor
	for pages := 0; ; pages++ {
		if pages > total { // safety net against an infinite loop on a bug
			t.Fatal("pagination did not terminate")
		}
		page, err := repo.ListPageForAccount(ctx, account.ID, nil, nil, false, after, pageSize)
		if err != nil {
			t.Fatalf("ListPageForAccount: %v", err)
		}
		if len(page) == 0 {
			break
		}
		got = append(got, page...)
		last := page[len(page)-1]
		after = &entities.OperationPageCursor{Date: last.Date, CreatedAt: last.CreatedAt, ID: last.ID}
		if len(page) < pageSize {
			break // last (partial) page
		}
	}

	if len(got) != len(want) {
		t.Fatalf("paginated through %d operations, want %d", len(got), len(want))
	}
	seen := map[uuid.UUID]bool{}
	for i, op := range got {
		if seen[op.ID] {
			t.Fatalf("operation %s returned more than once across pages", op.ID)
		}
		seen[op.ID] = true
		if op.ID != want[i].ID {
			t.Fatalf("page order mismatch at index %d: got %s, want %s", i, op.ID, want[i].ID)
		}
	}
}

func TestOperationRepository_ListPageForAccount_EmptyAccountReturnsNothing(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)

	page, err := NewOperationRepository(pool).ListPageForAccount(ctx, account.ID, nil, nil, false, nil, 50)
	if err != nil {
		t.Fatalf("ListPageForAccount: %v", err)
	}
	if len(page) != 0 {
		t.Fatalf("got %d operations, want 0", len(page))
	}
}

// An operation categorized either way — a plain category or splits — is
// left out by uncategorizedOnly.
func TestOperationRepository_ListPageForAccount_UncategorizedOnly(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	// Categories first so their cleanups run after the operations' ones
	// (t.Cleanup is LIFO), once nothing references them anymore.
	groceries := createTestCategory(t, pool)
	household := createTestCategory(t, pool)
	repo := NewOperationRepository(pool)

	uncategorized := mustCreateOperation(t, pool, account.ID, "Unknown shop", "-12.00")
	categorized := mustCreateOperation(t, pool, account.ID, "Carrefour", "-30.00")
	categorized.CategoryID = &groceries.ID
	if _, err := repo.Update(ctx, categorized); err != nil {
		t.Fatalf("Update: %v", err)
	}
	split := mustCreateOperation(t, pool, account.ID, "Supermarket", "-100.00")
	if _, err := NewOperationCategorySplitRepository(pool).ReplaceForOperation(ctx, split.ID, []entities.OperationCategorySplit{
		{CategoryID: groceries.ID, Amount: mustOpAmount(t, "70.00")},
		{CategoryID: household.ID, Amount: mustOpAmount(t, "30.00")},
	}); err != nil {
		t.Fatalf("ReplaceForOperation: %v", err)
	}

	page, err := repo.ListPageForAccount(ctx, account.ID, nil, nil, true, nil, 50)
	if err != nil {
		t.Fatalf("ListPageForAccount: %v", err)
	}
	if len(page) != 1 || page[0].ID != uncategorized.ID {
		t.Fatalf("got %d operations (%v), want only the uncategorized one", len(page), page)
	}

	all, err := repo.ListPageForAccount(ctx, account.ID, nil, nil, false, nil, 50)
	if err != nil {
		t.Fatalf("ListPageForAccount: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d operations without the filter, want 3", len(all))
	}
}

func TestOperationRepository_ListUnreconciled(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	user := createTestUser(t, pool)
	account := createTestAccount(t, pool, user.ID)
	item, err := NewPlannedItemRepository(pool).Add(ctx, &entities.PlannedItem{
		ID: uuid.New(), AccountID: account.ID, Label: strPtr("Rent"), Kind: entities.PlannedItemKindCharge,
		Amount: mustOpAmount(t, "800.00"), StartDate: civil.Today(), RecurrenceType: entities.RecurrencePerpetual, Active: true, MatchMode: entities.PlannedItemMatchModeLabel,})
	if err != nil {
		t.Fatalf("Add planned item: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_items WHERE id = $1", item.ID) })

	unreconciledOp := mustCreateOperation(t, pool, account.ID, "Unreconciled", "-1.00")
	reconciledOp := mustCreateOperation(t, pool, account.ID, "Reconciled", "-800.00")

	occ, err := NewPlannedOccurrenceRepository(pool).Add(ctx, &entities.PlannedOccurrence{
		ID: uuid.New(), PlannedItemID: item.ID, DueDate: civil.Today(),
		ExpectedAmount: mustOpAmount(t, "-800.00"), Status: entities.OccurrenceReconciled,
		ReconciledOperationID: &reconciledOp.ID,
	})
	if err != nil {
		t.Fatalf("Add occurrence: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM planned_occurrences WHERE id = $1", occ.ID) })

	unreconciled, err := NewOperationRepository(pool).ListUnreconciled(ctx, account.ID)
	if err != nil {
		t.Fatalf("ListUnreconciled: %v", err)
	}
	ids := map[uuid.UUID]bool{}
	for _, op := range unreconciled {
		ids[op.ID] = true
	}
	if !ids[unreconciledOp.ID] {
		t.Error("the operation with no reconciled occurrence should be listed as unreconciled")
	}
	if ids[reconciledOp.ID] {
		t.Error("the operation already linked from a reconciled occurrence should not be listed")
	}
}

// CategoryCountsForLabels is what powers cross-user auto-categorization
// learning (see services.AutoCategorizeUseCase) - it must aggregate across
// every account/user sharing a normalized label, not just the caller's own.
func TestOperationRepository_CategoryCountsForLabels(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewOperationRepository(pool)

	userA := createTestUser(t, pool)
	userB := createTestUser(t, pool)
	accountA := createTestAccount(t, pool, userA.ID)
	accountB := createTestAccount(t, pool, userB.ID)
	restaurants := createTestCategory(t, pool)
	groceries := createTestCategory(t, pool)

	label := "SHARED MERCHANT " + newTestUUIDString()
	normalized := normalizeForCounts(label)

	// User A categorized it as Restaurants twice.
	for i := 0; i < 2; i++ {
		op := mustCreateOperation(t, pool, accountA.ID, label, "-9.00")
		op.CategoryID = &restaurants.ID
		if _, err := repo.Update(ctx, op); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	// User B categorized it as Groceries once.
	opB := mustCreateOperation(t, pool, accountB.ID, label, "-9.00")
	opB.CategoryID = &groceries.ID
	if _, err := repo.Update(ctx, opB); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// An uncategorized operation with the same label must never appear in
	// the counts (only category_id IS NOT NULL rows count).
	_ = mustCreateOperation(t, pool, accountA.ID, label, "-9.00")

	counts, err := repo.CategoryCountsForLabels(ctx, []string{normalized})
	if err != nil {
		t.Fatalf("CategoryCountsForLabels: %v", err)
	}

	var gotA, gotB int
	for _, c := range counts {
		if c.NormalizedLabel != normalized {
			t.Fatalf("unexpected label in results: %q", c.NormalizedLabel)
		}
		switch {
		case c.OwnerUserID == userA.ID && c.CategoryID == restaurants.ID:
			gotA = c.Count
		case c.OwnerUserID == userB.ID && c.CategoryID == groceries.ID:
			gotB = c.Count
		default:
			t.Errorf("unexpected row: %+v", c)
		}
	}
	if gotA != 2 {
		t.Errorf("user A's Restaurants count = %d, want 2", gotA)
	}
	if gotB != 1 {
		t.Errorf("user B's Groceries count = %d, want 1", gotB)
	}
}

func TestOperationRepository_CategoryCountsForLabels_EmptyInput(t *testing.T) {
	pool := newTestPool(t)
	counts, err := NewOperationRepository(pool).CategoryCountsForLabels(context.Background(), nil)
	if err != nil {
		t.Fatalf("CategoryCountsForLabels: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("got %d counts, want 0 for an empty label list", len(counts))
	}
}

// normalizeForCounts mirrors the SQL's own lower(trim(label)), which is
// what CategoryCountsForLabels matches normalizedLabels against.
func normalizeForCounts(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}
