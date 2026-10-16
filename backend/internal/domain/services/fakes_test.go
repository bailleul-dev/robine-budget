package services

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

// In-memory fakes for the domain ports, mirroring
// backend/tests/domain/fakes.py — fast unit tests, no database.

type fakeUserRepo struct {
	byID map[uuid.UUID]*entities.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*entities.User{}}
}
func (f *fakeUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.User, error) {
	return f.byID[id], nil
}
func (f *fakeUserRepo) GetByEmail(ctx context.Context, email string) (*entities.User, error) {
	for _, u := range f.byID {
		if u.Email != nil && strings.EqualFold(*u.Email, email) {
			return u, nil
		}
	}
	return nil, nil
}
func (f *fakeUserRepo) GetByRobineSub(ctx context.Context, sub string) (*entities.User, error) {
	for _, u := range f.byID {
		if u.RobineSub != nil && *u.RobineSub == sub {
			return u, nil
		}
	}
	return nil, nil
}
func (f *fakeUserRepo) GetByGoogleSub(ctx context.Context, sub string) (*entities.User, error) {
	for _, u := range f.byID {
		if u.GoogleSub != nil && *u.GoogleSub == sub {
			return u, nil
		}
	}
	return nil, nil
}
func (f *fakeUserRepo) GetByAppleSub(ctx context.Context, sub string) (*entities.User, error) {
	for _, u := range f.byID {
		if u.AppleSub != nil && *u.AppleSub == sub {
			return u, nil
		}
	}
	return nil, nil
}
func (f *fakeUserRepo) Add(ctx context.Context, u *entities.User) (*entities.User, error) {
	f.byID[u.ID] = u
	return u, nil
}
func (f *fakeUserRepo) Update(ctx context.Context, u *entities.User) (*entities.User, error) {
	f.byID[u.ID] = u
	return u, nil
}

func (f *fakeUserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.byID, id)
	return nil
}

func (f *fakeUserRepo) BumpTokenVersion(ctx context.Context, id uuid.UUID) error {
	if u := f.byID[id]; u != nil {
		u.TokenVersion++
	}
	return nil
}

// fakePasswordHasher: reversible "hash" (prefixed plaintext) so tests can
// assert on Verify's behavior without pulling the real bcrypt adapter into
// the domain package's test dependencies.
type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(plain string) (string, error) {
	return "hashed:" + plain, nil
}
func (fakePasswordHasher) Verify(plain, hash string) bool {
	return hash == "hashed:"+plain
}

type fakePlannedItemRepo struct {
	byID map[uuid.UUID]*entities.PlannedItem
}

func newFakePlannedItemRepo() *fakePlannedItemRepo {
	return &fakePlannedItemRepo{byID: map[uuid.UUID]*entities.PlannedItem{}}
}
func (f *fakePlannedItemRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedItem, error) {
	return f.byID[id], nil
}
func (f *fakePlannedItemRepo) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.PlannedItem, error) {
	var out []entities.PlannedItem
	for _, i := range f.byID {
		if i.AccountID == accountID {
			out = append(out, *i)
		}
	}
	return out, nil
}
func (f *fakePlannedItemRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.PlannedItem, error) {
	return nil, nil
}
func (f *fakePlannedItemRepo) Add(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error) {
	f.byID[item.ID] = item
	return item, nil
}
func (f *fakePlannedItemRepo) Update(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error) {
	f.byID[item.ID] = item
	return item, nil
}

type fakeOccurrenceRepo struct {
	byID map[uuid.UUID]*entities.PlannedOccurrence
	// itemAccount: plannedItemID -> accountID, populated by tests that need
	// ListForAccount's account-scoped filtering (see
	// reconciliation_test.go's SuggestMatches tests) - the real DB
	// implementation resolves this with a join through planned_items,
	// which an occurrence has no direct account_id to filter by itself.
	itemAccount map[uuid.UUID]uuid.UUID
}

func newFakeOccurrenceRepo() *fakeOccurrenceRepo {
	return &fakeOccurrenceRepo{byID: map[uuid.UUID]*entities.PlannedOccurrence{}, itemAccount: map[uuid.UUID]uuid.UUID{}}
}
func (f *fakeOccurrenceRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedOccurrence, error) {
	return f.byID[id], nil
}
func (f *fakeOccurrenceRepo) ListForItem(ctx context.Context, plannedItemID uuid.UUID) ([]entities.PlannedOccurrence, error) {
	var out []entities.PlannedOccurrence
	for _, o := range f.byID {
		if o.PlannedItemID == plannedItemID {
			out = append(out, *o)
		}
	}
	return out, nil
}
func (f *fakeOccurrenceRepo) ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.PlannedOccurrence, error) {
	var out []entities.PlannedOccurrence
	for _, o := range f.byID {
		if f.itemAccount[o.PlannedItemID] != accountID {
			continue
		}
		if dateFrom != nil && o.DueDate.Before(*dateFrom) {
			continue
		}
		if dateTo != nil && o.DueDate.After(*dateTo) {
			continue
		}
		out = append(out, *o)
	}
	return out, nil
}
func (f *fakeOccurrenceRepo) GetByItemAndDate(ctx context.Context, plannedItemID uuid.UUID, dueDate civil.Date) (*entities.PlannedOccurrence, error) {
	for _, o := range f.byID {
		if o.PlannedItemID == plannedItemID && o.DueDate.Equal(dueDate) {
			return o, nil
		}
	}
	return nil, nil
}
func (f *fakeOccurrenceRepo) Add(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error) {
	f.byID[occ.ID] = occ
	return occ, nil
}
func (f *fakeOccurrenceRepo) AddMany(ctx context.Context, occs []entities.PlannedOccurrence) ([]entities.PlannedOccurrence, error) {
	for i := range occs {
		f.byID[occs[i].ID] = &occs[i]
	}
	return occs, nil
}
func (f *fakeOccurrenceRepo) Update(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error) {
	f.byID[occ.ID] = occ
	return occ, nil
}

type fakeOperationRepo struct {
	byID map[uuid.UUID]*entities.Operation
	// accountOwner: accountID -> owning user, populated by tests that
	// exercise CategoryCountsForLabels' cross-account/cross-user grouping
	// (see categorization_test.go). An account with no entry here counts as
	// owned by the zero UUID — fine for every other test, which never calls
	// that method at all.
	accountOwner map[uuid.UUID]uuid.UUID
	// reconciledIDs: operations already linked to some occurrence, set
	// directly by tests that need ListUnreconciled to exclude one (see
	// reconciliation_test.go) — the real implementation derives this from
	// planned_occurrences.reconciled_operation_id, which this fake has no
	// cross-reference into on its own.
	reconciledIDs map[uuid.UUID]bool
}

func newFakeOperationRepo() *fakeOperationRepo {
	return &fakeOperationRepo{
		byID:          map[uuid.UUID]*entities.Operation{},
		accountOwner:  map[uuid.UUID]uuid.UUID{},
		reconciledIDs: map[uuid.UUID]bool{},
	}
}
func (f *fakeOperationRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Operation, error) {
	return f.byID[id], nil
}
func (f *fakeOperationRepo) FindByReference(ctx context.Context, accountID uuid.UUID, ref string) (*entities.Operation, error) {
	for _, op := range f.byID {
		if op.AccountID == accountID && op.RawReference != nil && *op.RawReference == ref {
			return op, nil
		}
	}
	return nil, nil
}
func (f *fakeOperationRepo) FindByDedupHash(ctx context.Context, accountID uuid.UUID, hash string) (*entities.Operation, error) {
	for _, op := range f.byID {
		if op.AccountID == accountID && op.DedupHash == hash {
			return op, nil
		}
	}
	return nil, nil
}
func (f *fakeOperationRepo) Add(ctx context.Context, op *entities.Operation) (*entities.Operation, error) {
	f.byID[op.ID] = op
	return op, nil
}
func (f *fakeOperationRepo) AddMany(ctx context.Context, ops []entities.Operation) ([]entities.Operation, error) {
	for i := range ops {
		f.byID[ops[i].ID] = &ops[i]
	}
	return ops, nil
}
func (f *fakeOperationRepo) ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.Operation, error) {
	var out []entities.Operation
	for _, op := range f.byID {
		if op.AccountID != accountID {
			continue
		}
		if dateFrom != nil && op.Date.Before(*dateFrom) {
			continue
		}
		if dateTo != nil && op.Date.After(*dateTo) {
			continue
		}
		out = append(out, *op)
	}
	return out, nil
}
func (f *fakeOperationRepo) ListPageForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date, uncategorizedOnly bool, after *entities.OperationPageCursor, limit int) ([]entities.Operation, error) {
	return nil, nil // not exercised by these tests
}
func (f *fakeOperationRepo) ListUnreconciled(ctx context.Context, accountID uuid.UUID) ([]entities.Operation, error) {
	var out []entities.Operation
	for _, op := range f.byID {
		if op.AccountID != accountID || f.reconciledIDs[op.ID] {
			continue
		}
		out = append(out, *op)
	}
	return out, nil
}
func (f *fakeOperationRepo) Update(ctx context.Context, op *entities.Operation) (*entities.Operation, error) {
	f.byID[op.ID] = op
	return op, nil
}
func (f *fakeOperationRepo) CategoryCountsForLabels(ctx context.Context, normalizedLabels []string) ([]ports.LabelCategoryCount, error) {
	wanted := make(map[string]bool, len(normalizedLabels))
	for _, l := range normalizedLabels {
		wanted[l] = true
	}
	type key struct {
		label    string
		owner    uuid.UUID
		category uuid.UUID
	}
	counts := map[key]int{}
	for _, op := range f.byID {
		if op.CategoryID == nil {
			continue
		}
		label := normalizeLabel(op.Label)
		if !wanted[label] {
			continue
		}
		k := key{label: label, owner: f.accountOwner[op.AccountID], category: *op.CategoryID}
		counts[k]++
	}
	out := make([]ports.LabelCategoryCount, 0, len(counts))
	for k, n := range counts {
		out = append(out, ports.LabelCategoryCount{NormalizedLabel: k.label, OwnerUserID: k.owner, CategoryID: k.category, Count: n})
	}
	return out, nil
}

type fakeCategoryRepo struct {
	byID map[uuid.UUID]*entities.Category
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{byID: map[uuid.UUID]*entities.Category{}}
}
func (f *fakeCategoryRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Category, error) {
	return f.byID[id], nil
}
func (f *fakeCategoryRepo) ListVisibleTo(ctx context.Context, userID uuid.UUID) ([]entities.Category, error) {
	var out []entities.Category
	for _, c := range f.byID {
		if c.UserID == nil || *c.UserID == userID {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (f *fakeCategoryRepo) Add(ctx context.Context, c *entities.Category) (*entities.Category, error) {
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeCategoryRepo) Update(ctx context.Context, c *entities.Category) (*entities.Category, error) {
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeCategoryRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.byID, id)
	return nil
}

type fakeCheckpointRepo struct {
	byAccount map[uuid.UUID][]entities.BalanceCheckpoint
}

func newFakeCheckpointRepo() *fakeCheckpointRepo {
	return &fakeCheckpointRepo{byAccount: map[uuid.UUID][]entities.BalanceCheckpoint{}}
}
func (f *fakeCheckpointRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.BalanceCheckpoint, error) {
	return nil, nil
}
func (f *fakeCheckpointRepo) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.BalanceCheckpoint, error) {
	return f.byAccount[accountID], nil
}
func (f *fakeCheckpointRepo) GetLatestOnOrBefore(ctx context.Context, accountID uuid.UUID, asOf civil.Date) (*entities.BalanceCheckpoint, error) {
	var best *entities.BalanceCheckpoint
	for _, c := range f.byAccount[accountID] {
		c := c
		if c.AsOfDate.After(asOf) {
			continue
		}
		if best == nil || c.AsOfDate.After(best.AsOfDate) {
			best = &c
		}
	}
	return best, nil
}
func (f *fakeCheckpointRepo) Add(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	f.byAccount[c.AccountID] = append(f.byAccount[c.AccountID], *c)
	return c, nil
}
func (f *fakeCheckpointRepo) Upsert(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	for i, existing := range f.byAccount[c.AccountID] {
		if existing.AsOfDate.Equal(c.AsOfDate) {
			f.byAccount[c.AccountID][i] = *c
			return c, nil
		}
	}
	f.byAccount[c.AccountID] = append(f.byAccount[c.AccountID], *c)
	return c, nil
}
func (f *fakeCheckpointRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }

type fakeImportBatchRepo struct {
	byID map[uuid.UUID]*entities.ImportBatch
}

func newFakeImportBatchRepo() *fakeImportBatchRepo {
	return &fakeImportBatchRepo{byID: map[uuid.UUID]*entities.ImportBatch{}}
}
func (f *fakeImportBatchRepo) Add(ctx context.Context, b *entities.ImportBatch) (*entities.ImportBatch, error) {
	f.byID[b.ID] = b
	return b, nil
}
func (f *fakeImportBatchRepo) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.ImportBatch, error) {
	var out []entities.ImportBatch
	for _, b := range f.byID {
		if b.AccountID == accountID {
			out = append(out, *b)
		}
	}
	return out, nil
}

type fakeCollaboratorRepo struct {
	byID map[uuid.UUID]*entities.Collaborator
}

func newFakeCollaboratorRepo() *fakeCollaboratorRepo {
	return &fakeCollaboratorRepo{byID: map[uuid.UUID]*entities.Collaborator{}}
}
func (f *fakeCollaboratorRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Collaborator, error) {
	return f.byID[id], nil
}
func (f *fakeCollaboratorRepo) GetByToken(ctx context.Context, token string) (*entities.Collaborator, error) {
	for _, c := range f.byID {
		if c.Token == token {
			return c, nil
		}
	}
	return nil, nil
}
func (f *fakeCollaboratorRepo) ListForOwner(ctx context.Context, ownerUserID uuid.UUID) ([]entities.Collaborator, error) {
	var out []entities.Collaborator
	for _, c := range f.byID {
		if c.OwnerUserID == ownerUserID {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (f *fakeCollaboratorRepo) ListActiveForCollaborator(ctx context.Context, collaboratorUserID uuid.UUID) ([]entities.Collaborator, error) {
	var out []entities.Collaborator
	for _, c := range f.byID {
		if c.CollaboratorUserID != nil && *c.CollaboratorUserID == collaboratorUserID && c.AcceptedAt != nil && c.RevokedAt == nil {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (f *fakeCollaboratorRepo) Add(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error) {
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeCollaboratorRepo) Update(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error) {
	f.byID[c.ID] = c
	return c, nil
}

func amt(s string) money.Amount {
	a, err := money.New(s)
	if err != nil {
		panic(err)
	}
	return a
}
