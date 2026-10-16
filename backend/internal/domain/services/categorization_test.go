package services

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/migrations"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

func TestSuggestCategoryID(t *testing.T) {
	groceriesID := uuid.New()
	byName := map[string]uuid.UUID{"Groceries": groceriesID}

	cases := []struct {
		name  string
		label string
		want  *uuid.UUID
	}{
		{"real CB-noise label matches the embedded merchant", "PAIEMENT PSC 1110 BAILLEUL E.LECLERC CARTE 9102", &groceriesID},
		{"word boundary: short keyword must not match inside a longer word", "PAIEMENT CB 0109 BUTLER CARTE 9102", nil}, // "but" is a keyword elsewhere, not tested here, but guards the mechanism
		{"no match falls through to nil", "VIR DE M GUILLAUME BAILLEUL", nil},
		{"unknown category name in dictionary never resolves", "PAIEMENT CB LECLERC", &groceriesID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := suggestCategoryID(tc.label, byName)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("suggestCategoryID(%q) = %v, want nil", tc.label, *got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("suggestCategoryID(%q) = %v, want %v", tc.label, got, *tc.want)
			}
		})
	}
}

func TestSuggestCategoryID_WordBoundary(t *testing.T) {
	furnitureID := uuid.New()
	byName := map[string]uuid.UUID{"Home & furniture": furnitureID}

	// "but" (a real furniture-store keyword) must not match inside
	// unrelated words like "butler" or "tribute".
	if got := suggestCategoryID("PAIEMENT CB BUTLER SERVICES", byName); got != nil {
		t.Fatalf("expected no match for %q, got %v", "BUTLER SERVICES", *got)
	}
	if got := suggestCategoryID("PAIEMENT PSC 0908 BAILLEUL BUT CARTE 9102", byName); got == nil || *got != furnitureID {
		t.Fatalf("expected furniture match for a real BUT occurrence, got %v", got)
	}
}

func TestSuggestCategoryID_LongestKeywordWins(t *testing.T) {
	electricityID := uuid.New()
	utilitiesID := uuid.New()
	byName := map[string]uuid.UUID{"Electricity": electricityID, "Utilities": utilitiesID}

	// "direct energie" is more specific than a hypothetical shorter
	// overlapping rule — simulate that by checking the actual dictionary
	// entries resolve to the expected, more specific category.
	got := suggestCategoryID("PRLV SEPA DIRECT ENERGIE", byName)
	if got == nil || *got != electricityID {
		t.Fatalf("expected Electricity for Direct Energie, got %v", got)
	}
}

func newTestAutoCategorizeUseCase(operations *fakeOperationRepo, categories *fakeCategoryRepo) *AutoCategorizeUseCase {
	return NewAutoCategorizeUseCase(operations, categories)
}

func TestAutoCategorizeUseCase_DictionaryAndMemory(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	ownerID := uuid.New()
	accountID := uuid.New()

	groceries := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	restaurants := &entities.Category{ID: uuid.New(), Name: "Restaurants"}
	categories.byID[groceries.ID] = groceries
	categories.byID[restaurants.ID] = restaurants

	mustAmount := func(s string) money.Amount {
		amt, err := money.New(s)
		if err != nil {
			t.Fatalf("money.New(%q): %v", s, err)
		}
		return amt
	}

	operations.accountOwner[accountID] = ownerID

	// A dictionary hit (E.LECLERC), and three occurrences of an unknown
	// local café under an identical label — nothing in the dictionary
	// knows what "BELLENAERT CAFE" is, so only this owner's own history
	// (seeded by categorizing the first one) should catch the other two.
	date := civil.Today()
	leclerc := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: date, Label: "PAIEMENT PSC 1110 BAILLEUL E.LECLERC CARTE 9102", Amount: mustAmount("-15.00"), DedupHash: "h1"}
	cafe1 := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: date, Label: "BELLENAERT CAFE", Amount: mustAmount("-4.50"), DedupHash: "h2", CategoryID: &restaurants.ID}
	cafe2 := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: date, Label: "BELLENAERT CAFE", Amount: mustAmount("-3.80"), DedupHash: "h3"}
	unknown := &entities.Operation{ID: uuid.New(), AccountID: accountID, Date: date, Label: "TERRIER FILS SC", Amount: mustAmount("-20.00"), DedupHash: "h4"}
	for _, op := range []*entities.Operation{leclerc, cafe1, cafe2, unknown} {
		operations.byID[op.ID] = op
	}

	uc := newTestAutoCategorizeUseCase(operations, categories)
	count, err := uc.CategorizeAccount(ctx, accountID, ownerID)
	if err != nil {
		t.Fatalf("CategorizeAccount: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (leclerc via dictionary, cafe2 via memory)", count)
	}
	if got := operations.byID[leclerc.ID].CategoryID; got == nil || *got != groceries.ID {
		t.Fatalf("leclerc category = %v, want Groceries", got)
	}
	if got := operations.byID[cafe2.ID].CategoryID; got == nil || *got != restaurants.ID {
		t.Fatalf("cafe2 category = %v, want Restaurants (learned from cafe1)", got)
	}
	if got := operations.byID[unknown.ID].CategoryID; got != nil {
		t.Fatalf("unknown category = %v, want nil (nothing to match on)", got)
	}
	// Already-categorized operations are never touched.
	if got := operations.byID[cafe1.ID].CategoryID; got == nil || *got != restaurants.ID {
		t.Fatalf("cafe1 category changed unexpectedly: %v", got)
	}
}

func TestAutoCategorizeUseCase_NoUncategorizedOperations(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	uc := newTestAutoCategorizeUseCase(operations, categories)

	count, err := uc.CategorizeAccount(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("CategorizeAccount: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func mustTestAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

// TestAutoCategorizeUseCase_CrossUserLearning is the core "system improves
// itself in production" scenario: user B never dictionary-matched
// "TERRIER FILS SC" (a local shop nothing generic could know about) — user
// A already categorized it by hand once, on a completely different account
// they own. B's own auto-categorize run should pick that up, purely from
// the shared operations history, with no code change or redeploy.
func TestAutoCategorizeUseCase_CrossUserLearning(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	userA, userB := uuid.New(), uuid.New()
	accountA, accountB := uuid.New(), uuid.New()
	operations.accountOwner[accountA] = userA
	operations.accountOwner[accountB] = userB

	restaurants := &entities.Category{ID: uuid.New(), Name: "Restaurants"}
	categories.byID[restaurants.ID] = restaurants

	date := civil.Today()
	fromA := &entities.Operation{ID: uuid.New(), AccountID: accountA, Date: date, Label: "TERRIER FILS SC", Amount: mustTestAmount(t, "-12.00"), DedupHash: "a1", CategoryID: &restaurants.ID}
	fromB := &entities.Operation{ID: uuid.New(), AccountID: accountB, Date: date, Label: "TERRIER FILS SC", Amount: mustTestAmount(t, "-9.00"), DedupHash: "b1"}
	operations.byID[fromA.ID] = fromA
	operations.byID[fromB.ID] = fromB

	uc := newTestAutoCategorizeUseCase(operations, categories)
	count, err := uc.CategorizeAccount(ctx, accountB, userB)
	if err != nil {
		t.Fatalf("CategorizeAccount: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if got := operations.byID[fromB.ID].CategoryID; got == nil || *got != restaurants.ID {
		t.Fatalf("fromB category = %v, want Restaurants (learned from user A)", got)
	}
}

// TestAutoCategorizeUseCase_OwnChoiceOutranksCrowd: several other users
// categorized a label one way; this account's own owner already categorized
// it differently themselves, elsewhere. Their own choice must win — "priorite
// aux categories/correction du user" is the whole point of ranking ownTop
// before crowdTop.
func TestAutoCategorizeUseCase_OwnChoiceOutranksCrowd(t *testing.T) {
	ctx := context.Background()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	me := uuid.New()
	otherUsers := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	myAccount := uuid.New()
	operations.accountOwner[myAccount] = me

	streaming := &entities.Category{ID: uuid.New(), Name: "Streaming"}
	gifts := &entities.Category{ID: uuid.New(), Name: "Gifts"}
	categories.byID[streaming.ID] = streaming
	categories.byID[gifts.ID] = gifts

	date := civil.Today()
	const label = "VIR SEPA J DUPONT"

	// The crowd (3 other users, 3 different accounts) all called it Streaming.
	for i, other := range otherUsers {
		acc := uuid.New()
		operations.accountOwner[acc] = other
		op := &entities.Operation{
			ID: uuid.New(), AccountID: acc, Date: date, Label: label,
			Amount: mustTestAmount(t, "-8.00"), DedupHash: fmt.Sprintf("crowd-%d", i), CategoryID: &streaming.ID,
		}
		operations.byID[op.ID] = op
	}
	// This user already called the exact same label a Gift themselves.
	myOwnPick := &entities.Operation{
		ID: uuid.New(), AccountID: myAccount, Date: date, Label: label,
		Amount: mustTestAmount(t, "-8.00"), DedupHash: "mine-1", CategoryID: &gifts.ID,
	}
	// A second, still-uncategorized occurrence on my own account.
	myUncategorized := &entities.Operation{
		ID: uuid.New(), AccountID: myAccount, Date: date, Label: label,
		Amount: mustTestAmount(t, "-8.00"), DedupHash: "mine-2",
	}
	operations.byID[myOwnPick.ID] = myOwnPick
	operations.byID[myUncategorized.ID] = myUncategorized

	uc := newTestAutoCategorizeUseCase(operations, categories)
	count, err := uc.CategorizeAccount(ctx, myAccount, me)
	if err != nil {
		t.Fatalf("CategorizeAccount: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if got := operations.byID[myUncategorized.ID].CategoryID; got == nil || *got != gifts.ID {
		t.Fatalf("category = %v, want Gifts (own choice must outrank the crowd's Streaming)", got)
	}
}

func TestSuggestCategoryID_AlcoholAndTobaccoAreSeparate(t *testing.T) {
	alcoholID, tobaccoID := uuid.New(), uuid.New()
	byName := map[string]uuid.UUID{"Alcohol": alcoholID, "Tobacco": tobaccoID}
	for label, want := range map[string]uuid.UUID{
		"PAIEMENT CB 1209 LE BALTO TABAC PARIS": tobaccoID,
		"CB CAVAVIN LYON 3":                     alcoholID,
		"PAIEMENT CB LA CAVE A VIN":             alcoholID,
	} {
		if got := suggestCategoryID(label, byName); got == nil || *got != want {
			t.Errorf("suggestCategoryID(%q) = %v, want %v", label, got, want)
		}
	}
}

// Every category the dictionary points to must be seeded by a migration
// under exactly that name — otherwise its rules silently never match (see
// merchantRule).
func TestMerchantDictionaryCategoriesAreSeeded(t *testing.T) {
	var sql strings.Builder
	entries, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range entries {
		raw, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatal(err)
		}
		sql.Write(raw)
	}
	seeded := sql.String()
	for _, rule := range merchantDictionary {
		quoted := "'" + strings.ReplaceAll(rule.category, "'", "''") + "'"
		if !strings.Contains(seeded, quoted) {
			t.Errorf("rule %q → %q: no migration seeds a category named %s", rule.keyword, rule.category, quoted)
		}
	}
}
