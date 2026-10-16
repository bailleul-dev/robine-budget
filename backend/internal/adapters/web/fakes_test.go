package web

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
)

// In-memory fakes for the domain ports, mirroring the convention already
// used in internal/domain/services/fakes_test.go — fast unit tests, no
// database. Handlers are exercised directly (not through the router or
// AuthMiddleware): tests set the authenticated user on the request context
// themselves via userCtxKey.

func strPtr(s string) *string { return &s }

type fakeUserRepo struct {
	byID map[uuid.UUID]*entities.User
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{byID: map[uuid.UUID]*entities.User{}} }

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

type fakeAccountRepo struct {
	byID map[uuid.UUID]*entities.Account
	// err, when set, is returned by every method instead - lets tests
	// exercise a handler's "internal server error" branch.
	err error
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{byID: map[uuid.UUID]*entities.Account{}}
}
func (f *fakeAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byID[id], nil
}
func (f *fakeAccountRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []entities.Account
	for _, a := range f.byID {
		if a.UserID == userID {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (f *fakeAccountRepo) GetByBankSyncAccountID(ctx context.Context, userID uuid.UUID, bankSyncAccountID string) (*entities.Account, error) {
	for _, a := range f.byID {
		if a.UserID == userID && a.BankSyncAccountID != nil && *a.BankSyncAccountID == bankSyncAccountID {
			return a, nil
		}
	}
	return nil, nil
}
func (f *fakeAccountRepo) Add(ctx context.Context, a *entities.Account) (*entities.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.byID[a.ID] = a
	return a, nil
}
func (f *fakeAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	delete(f.byID, id)
	return nil
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
		if c.CollaboratorUserID != nil && *c.CollaboratorUserID == collaboratorUserID &&
			c.AcceptedAt != nil && c.RevokedAt == nil {
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

type fakeCategoryRepo struct {
	byID map[uuid.UUID]*entities.Category
	// deleteErr, when set, is returned by Delete instead of actually
	// deleting - lets tests simulate a foreign-key violation (subcategories
	// still pointing at the category being deleted).
	deleteErr error
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
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.byID, id)
	return nil
}

type fakeOperationRepo struct {
	byID map[uuid.UUID]*entities.Operation
	// reconciledIDs marks operations already linked from a reconciled
	// occurrence - ListUnreconciled excludes them.
	reconciledIDs map[uuid.UUID]bool
}

func newFakeOperationRepo() *fakeOperationRepo {
	return &fakeOperationRepo{byID: map[uuid.UUID]*entities.Operation{}, reconciledIDs: map[uuid.UUID]bool{}}
}
func (f *fakeOperationRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Operation, error) {
	return f.byID[id], nil
}
func (f *fakeOperationRepo) FindByReference(ctx context.Context, accountID uuid.UUID, rawReference string) (*entities.Operation, error) {
	for _, o := range f.byID {
		if o.AccountID == accountID && o.RawReference != nil && *o.RawReference == rawReference {
			return o, nil
		}
	}
	return nil, nil
}
func (f *fakeOperationRepo) FindByDedupHash(ctx context.Context, accountID uuid.UUID, dedupHash string) (*entities.Operation, error) {
	for _, o := range f.byID {
		if o.AccountID == accountID && o.DedupHash == dedupHash {
			return o, nil
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
	for _, o := range f.byID {
		if o.AccountID != accountID {
			continue
		}
		if dateFrom != nil && o.Date.Before(*dateFrom) {
			continue
		}
		if dateTo != nil && o.Date.After(*dateTo) {
			continue
		}
		out = append(out, *o)
	}
	return out, nil
}
func (f *fakeOperationRepo) ListPageForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date, uncategorizedOnly bool, after *entities.OperationPageCursor, limit int) ([]entities.Operation, error) {
	all, _ := f.ListForAccount(ctx, accountID, dateFrom, dateTo)
	if uncategorizedOnly {
		kept := all[:0]
		for _, o := range all {
			if o.CategoryID == nil {
				kept = append(kept, o)
			}
		}
		all = kept
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
func (f *fakeOperationRepo) ListUnreconciled(ctx context.Context, accountID uuid.UUID) ([]entities.Operation, error) {
	var out []entities.Operation
	for _, o := range f.byID {
		if o.AccountID == accountID && !f.reconciledIDs[o.ID] {
			out = append(out, *o)
		}
	}
	return out, nil
}
func (f *fakeOperationRepo) Update(ctx context.Context, op *entities.Operation) (*entities.Operation, error) {
	f.byID[op.ID] = op
	return op, nil
}
func (f *fakeOperationRepo) CategoryCountsForLabels(ctx context.Context, normalizedLabels []string) ([]ports.LabelCategoryCount, error) {
	return nil, nil
}

type fakeSplitRepo struct {
	byOperation map[uuid.UUID][]entities.OperationCategorySplit
}

func newFakeSplitRepo() *fakeSplitRepo {
	return &fakeSplitRepo{byOperation: map[uuid.UUID][]entities.OperationCategorySplit{}}
}
func (f *fakeSplitRepo) ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.OperationCategorySplit, error) {
	return f.byOperation[operationID], nil
}
func (f *fakeSplitRepo) ListForOperationIDs(ctx context.Context, operationIDs []uuid.UUID) ([]entities.OperationCategorySplit, error) {
	var out []entities.OperationCategorySplit
	for _, id := range operationIDs {
		out = append(out, f.byOperation[id]...)
	}
	return out, nil
}
func (f *fakeSplitRepo) ReplaceForOperation(ctx context.Context, operationID uuid.UUID, splits []entities.OperationCategorySplit) ([]entities.OperationCategorySplit, error) {
	for i := range splits {
		splits[i].ID = uuid.New()
		splits[i].OperationID = operationID
	}
	f.byOperation[operationID] = splits
	return splits, nil
}

type fakeCommentRepo struct {
	byOperation map[uuid.UUID][]entities.Comment
}

func newFakeCommentRepo() *fakeCommentRepo {
	return &fakeCommentRepo{byOperation: map[uuid.UUID][]entities.Comment{}}
}
func (f *fakeCommentRepo) ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.Comment, error) {
	return f.byOperation[operationID], nil
}
func (f *fakeCommentRepo) ListForAuthor(ctx context.Context, userID uuid.UUID) ([]entities.Comment, error) {
	var out []entities.Comment
	for _, cs := range f.byOperation {
		for _, c := range cs {
			if c.UserID == userID {
				out = append(out, c)
			}
		}
	}
	return out, nil
}
func (f *fakeCommentRepo) Add(ctx context.Context, c *entities.Comment) (*entities.Comment, error) {
	f.byOperation[c.OperationID] = append(f.byOperation[c.OperationID], *c)
	return c, nil
}

type fakeBalanceCheckpointRepo struct {
	byID map[uuid.UUID]*entities.BalanceCheckpoint
}

func newFakeBalanceCheckpointRepo() *fakeBalanceCheckpointRepo {
	return &fakeBalanceCheckpointRepo{byID: map[uuid.UUID]*entities.BalanceCheckpoint{}}
}
func (f *fakeBalanceCheckpointRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.BalanceCheckpoint, error) {
	return f.byID[id], nil
}
func (f *fakeBalanceCheckpointRepo) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.BalanceCheckpoint, error) {
	var out []entities.BalanceCheckpoint
	for _, c := range f.byID {
		if c.AccountID == accountID {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (f *fakeBalanceCheckpointRepo) GetLatestOnOrBefore(ctx context.Context, accountID uuid.UUID, asOf civil.Date) (*entities.BalanceCheckpoint, error) {
	var latest *entities.BalanceCheckpoint
	for _, c := range f.byID {
		if c.AccountID != accountID || c.AsOfDate.After(asOf) {
			continue
		}
		if latest == nil || c.AsOfDate.After(latest.AsOfDate) {
			cp := *c
			latest = &cp
		}
	}
	return latest, nil
}
func (f *fakeBalanceCheckpointRepo) Add(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeBalanceCheckpointRepo) Upsert(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	for _, existing := range f.byID {
		if existing.AccountID == c.AccountID && existing.AsOfDate.Equal(c.AsOfDate) {
			existing.Balance = c.Balance
			existing.Note = c.Note
			return existing, nil
		}
	}
	f.byID[c.ID] = c
	return c, nil
}
func (f *fakeBalanceCheckpointRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.byID, id)
	return nil
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
	byID        map[uuid.UUID]*entities.PlannedOccurrence
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

type fakeBankProfileRepo struct {
	byID map[uuid.UUID]*entities.BankProfile
}

func newFakeBankProfileRepo() *fakeBankProfileRepo {
	return &fakeBankProfileRepo{byID: map[uuid.UUID]*entities.BankProfile{}}
}
func (f *fakeBankProfileRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.BankProfile, error) {
	return f.byID[id], nil
}
func (f *fakeBankProfileRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.BankProfile, error) {
	var out []entities.BankProfile
	for _, p := range f.byID {
		if p.UserID == nil || *p.UserID == userID {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (f *fakeBankProfileRepo) Add(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	f.byID[p.ID] = p
	return p, nil
}
func (f *fakeBankProfileRepo) Update(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	f.byID[p.ID] = p
	return p, nil
}

type fakeImportBatchRepo struct {
	byAccount map[uuid.UUID][]entities.ImportBatch
}

func newFakeImportBatchRepo() *fakeImportBatchRepo {
	return &fakeImportBatchRepo{byAccount: map[uuid.UUID][]entities.ImportBatch{}}
}
func (f *fakeImportBatchRepo) Add(ctx context.Context, b *entities.ImportBatch) (*entities.ImportBatch, error) {
	f.byAccount[b.AccountID] = append(f.byAccount[b.AccountID], *b)
	return b, nil
}
func (f *fakeImportBatchRepo) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.ImportBatch, error) {
	return f.byAccount[accountID], nil
}

type fakeBankConnectionRepo struct {
	byUser map[uuid.UUID]*entities.BankConnection
}

func newFakeBankConnectionRepo() *fakeBankConnectionRepo {
	return &fakeBankConnectionRepo{byUser: map[uuid.UUID]*entities.BankConnection{}}
}
func (f *fakeBankConnectionRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*entities.BankConnection, error) {
	return f.byUser[userID], nil
}
func (f *fakeBankConnectionRepo) Add(ctx context.Context, c *entities.BankConnection) (*entities.BankConnection, error) {
	f.byUser[c.UserID] = c
	return c, nil
}
func (f *fakeBankConnectionRepo) UpdateSession(ctx context.Context, id uuid.UUID, sessionEncrypted []byte) error {
	for _, c := range f.byUser {
		if c.ID == id {
			c.SessionEncrypted = sessionEncrypted
		}
	}
	return nil
}
func (f *fakeBankConnectionRepo) ListUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(f.byUser))
	for userID := range f.byUser {
		ids = append(ids, userID)
	}
	return ids, nil
}

// fakeBankConnector returns canned accounts/err for
// ports.BankSyncConnector - these handler tests care about wiring, not real
// woob/bank_sync behavior (see internal/domain/services for that).
type fakeBankConnector struct {
	accounts        []entities.BankSyncAccount
	sessionToReturn []byte
	err             error
}

func (f *fakeBankConnector) FetchAccounts(ctx context.Context, login, password string, session []byte) ([]entities.BankSyncAccount, []byte, error) {
	if f.err != nil {
		return nil, f.sessionToReturn, f.err
	}
	return f.accounts, f.sessionToReturn, nil
}

// identityEncryptor round-trips bytes unchanged.
type identityEncryptor struct{}

func (identityEncryptor) Encrypt(p []byte) ([]byte, error) { return append([]byte(nil), p...), nil }
func (identityEncryptor) Decrypt(c []byte) ([]byte, error) { return append([]byte(nil), c...), nil }

type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (fakePasswordHasher) Verify(plain, hash string) bool    { return hash == "hashed:"+plain }

type fakeClock struct {
	today civil.Date
}

func newFakeClock(today civil.Date) *fakeClock { return &fakeClock{today: today} }
func (c *fakeClock) Today() civil.Date         { return c.today }
func (c *fakeClock) Now() time.Time            { return c.today.Time }
