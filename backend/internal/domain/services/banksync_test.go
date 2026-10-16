package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

// --- fakes -----------------------------------------------------------

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

type fakeAccountRepo struct {
	byID map[uuid.UUID]*entities.Account
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{byID: map[uuid.UUID]*entities.Account{}}
}
func (f *fakeAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*entities.Account, error) {
	return f.byID[id], nil
}
func (f *fakeAccountRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.Account, error) {
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
	f.byID[a.ID] = a
	return a, nil
}
func (f *fakeAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(f.byID, id)
	return nil
}

// fakeConnector returns sessionToReturn alongside the canned accounts/err -
// lets tests assert the session round trip without touching woob at all.
type fakeConnector struct {
	accounts        []entities.BankSyncAccount
	sessionToReturn []byte
	err             error
}

func (f *fakeConnector) FetchAccounts(ctx context.Context, login, password string, session []byte) ([]entities.BankSyncAccount, []byte, error) {
	if f.err != nil {
		return nil, f.sessionToReturn, f.err
	}
	return f.accounts, f.sessionToReturn, nil
}

// identityEncryptor round-trips bytes unchanged - these tests care about
// wiring (what gets decrypted/encrypted and when), not real cryptography.
type identityEncryptor struct{}

func (identityEncryptor) Encrypt(p []byte) ([]byte, error) { return append([]byte(nil), p...), nil }
func (identityEncryptor) Decrypt(c []byte) ([]byte, error) { return append([]byte(nil), c...), nil }

func newTestSyncUseCase(t *testing.T, connections *fakeBankConnectionRepo, accounts *fakeAccountRepo, checkpoints *fakeCheckpointRepo, operations *fakeOperationRepo, connector *fakeConnector) *SyncBankAccountsUseCase {
	t.Helper()
	clock := fixedClock{now: time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)}
	autoCategorize := NewAutoCategorizeUseCase(operations, newFakeCategoryRepo())
	return NewSyncBankAccountsUseCase(connections, accounts, checkpoints, operations, connector, identityEncryptor{}, clock, autoCategorize)
}

// --- SyncBankAccountsUseCase -------------------------------------------

func TestSyncBankAccounts_CreatesAccountAndRecordsBalance(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()
	connector := &fakeConnector{accounts: []entities.BankSyncAccount{
		{ID: "ext-1", Label: "Compte courant", Balance: amt("123.45"), Currency: "EUR"},
	}}
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)

	userID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: uuid.New(), UserID: userID,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	}

	result, err := uc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result) != 1 || !result[0].Created {
		t.Fatalf("expected one newly created account, got %+v", result)
	}
	account := result[0].Account
	if account.BankSyncAccountID == nil || *account.BankSyncAccountID != "ext-1" {
		t.Fatalf("expected account linked to ext-1, got %v", account.BankSyncAccountID)
	}

	saved, _ := checkpoints.ListForAccount(context.Background(), account.ID)
	if len(saved) != 1 || saved[0].Balance.StringFixed(2) != "123.45" {
		t.Fatalf("expected one checkpoint of 123.45, got %+v", saved)
	}
}

// TestSyncBankAccounts_SavesOperationsAndDedupesAcrossSyncs mirrors what
// ImportCsvUseCase's dedup tests check for CSV rows, but for bank_sync:
// the first sync must save every transaction as an Operation, and a second
// sync whose window overlaps the first must not duplicate the ones already
// saved - only genuinely new transactions should add rows.
func TestSyncBankAccounts_SavesOperationsAndDedupesAcrossSyncs(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()

	userID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: uuid.New(), UserID: userID,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	}

	txn1 := entities.BankSyncTransaction{ID: "txn-1", Date: civil.New(2026, time.March, 1), Label: "Loyer", Amount: amt("-800.00")}
	txn2 := entities.BankSyncTransaction{ID: "txn-2", Date: civil.New(2026, time.March, 2), Label: "Salaire", Amount: amt("2000.00")}
	connector := &fakeConnector{accounts: []entities.BankSyncAccount{
		{ID: "ext-1", Label: "Compte courant", Balance: amt("1200.00"), Currency: "EUR", Operations: []entities.BankSyncTransaction{txn1, txn2}},
	}}
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)

	result, err := uc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if result[0].OperationsAdded != 2 {
		t.Fatalf("expected 2 operations added on first sync, got %d", result[0].OperationsAdded)
	}
	if len(operations.byID) != 2 {
		t.Fatalf("expected 2 operations stored, got %d", len(operations.byID))
	}

	// Second sync: txn2 comes back again (overlapping window) plus a
	// genuinely new txn3 - only txn3 should be added.
	txn3 := entities.BankSyncTransaction{ID: "txn-3", Date: civil.New(2026, time.March, 3), Label: "Courses", Amount: amt("-45.30")}
	connector.accounts[0].Operations = []entities.BankSyncTransaction{txn2, txn3}

	result, err = uc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result[0].OperationsAdded != 1 {
		t.Fatalf("expected 1 new operation added on second sync (txn3 only), got %d", result[0].OperationsAdded)
	}
	if len(operations.byID) != 3 {
		t.Fatalf("expected 3 operations stored total, got %d", len(operations.byID))
	}
}

func TestSyncBankAccounts_ReusesLinkedAccountAndUpsertsSameDayCheckpoint(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()

	userID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: uuid.New(), UserID: userID,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	}
	bankSyncAccountID := "ext-1"
	existing := &entities.Account{ID: uuid.New(), UserID: userID, Name: "Compte courant", Currency: "EUR", BankSyncAccountID: &bankSyncAccountID}
	accounts.byID[existing.ID] = existing

	connector := &fakeConnector{accounts: []entities.BankSyncAccount{
		{ID: "ext-1", Label: "Compte courant", Balance: amt("100.00"), Currency: "EUR"},
	}}
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)

	if _, err := uc.Sync(context.Background(), userID); err != nil {
		t.Fatalf("first Sync: %v", err)
	}

	// Second sync same day, balance moved - must update the same
	// checkpoint row, not add a second one for the same date.
	connector.accounts[0].Balance = amt("90.00")
	result, err := uc.Sync(context.Background(), userID)
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if len(result) != 1 || result[0].Created {
		t.Fatalf("expected the existing account reused (not created), got %+v", result)
	}
	if result[0].Account.ID != existing.ID {
		t.Fatalf("expected the same account %s, got %s", existing.ID, result[0].Account.ID)
	}

	saved, _ := checkpoints.ListForAccount(context.Background(), existing.ID)
	if len(saved) != 1 || saved[0].Balance.StringFixed(2) != "90.00" {
		t.Fatalf("expected a single checkpoint updated to 90.00, got %+v", saved)
	}
}

func TestSyncBankAccounts_SavesSessionOnlyWhenChanged(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()

	userID := uuid.New()
	connID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: connID, UserID: userID,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
		SessionEncrypted: []byte("old-session"),
	}

	connector := &fakeConnector{sessionToReturn: []byte("old-session")} // unchanged
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)
	if _, err := uc.Sync(context.Background(), userID); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if string(connections.byUser[userID].SessionEncrypted) != "old-session" {
		t.Fatalf("session should be untouched when unchanged, got %q", connections.byUser[userID].SessionEncrypted)
	}

	connector.sessionToReturn = []byte("new-session") // changed this time
	if _, err := uc.Sync(context.Background(), userID); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if string(connections.byUser[userID].SessionEncrypted) != "new-session" {
		t.Fatalf("expected session to be saved as new-session, got %q", connections.byUser[userID].SessionEncrypted)
	}
}

// TestSyncBankAccounts_SavesSessionEvenWhenFetchFails covers the port-style
// contract's error path: bank_sync/sync.py's fail() reports whatever
// session state woob left behind even when the call itself raised (e.g. a
// 2FA wait that timed out) - that progress must still reach the DB so a
// retry doesn't re-trigger the app-validation prompt from scratch.
func TestSyncBankAccounts_SavesSessionEvenWhenFetchFails(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()

	userID := uuid.New()
	connID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: connID, UserID: userID,
		LoginEncrypted: []byte("login"), PasswordEncrypted: []byte("password"),
	}

	connector := &fakeConnector{
		sessionToReturn: []byte("partial-2fa-progress"),
		err:             fmt.Errorf("bank_sync: two_factor_timeout: Delai depasse"),
	}
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)

	_, err := uc.Sync(context.Background(), userID)
	var syncErr *BankSyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected a *BankSyncError, got %v (%T)", err, err)
	}
	if string(connections.byUser[userID].SessionEncrypted) != "partial-2fa-progress" {
		t.Fatalf("expected the partial session to be saved despite the error, got %q", connections.byUser[userID].SessionEncrypted)
	}
}

func TestSyncBankAccounts_NoConnection_ReturnsBankSyncError(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, &fakeConnector{})

	_, err := uc.Sync(context.Background(), uuid.New())
	var syncErr *BankSyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected a *BankSyncError, got %v (%T)", err, err)
	}
}

// TestSyncBankAccounts_ConnectorErrorNeverLeaksCredentials guards the "must
// never return plaintext credentials" requirement: even a (deliberately
// worst-case) connector error that echoes the login/password back must come
// out fully redacted, since *BankSyncError.Error() is exactly the string an
// HTTP handler puts in the response body.
func TestSyncBankAccounts_ConnectorErrorNeverLeaksCredentials(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()

	userID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{
		ID: uuid.New(), UserID: userID,
		LoginEncrypted: []byte("s3cr3t-login"), PasswordEncrypted: []byte("s3cr3t-password"),
	}
	connector := &fakeConnector{err: fmt.Errorf("auth failed for s3cr3t-login / s3cr3t-password")}
	uc := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)

	_, err := uc.Sync(context.Background(), userID)
	var syncErr *BankSyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected a *BankSyncError, got %v (%T)", err, err)
	}
	msg := syncErr.Error()
	if strings.Contains(msg, "s3cr3t-login") || strings.Contains(msg, "s3cr3t-password") {
		t.Fatalf("BankSyncError leaked a plaintext credential: %q", msg)
	}
}

// --- ConnectBankUseCase --------------------------------------------------

func TestConnectBank_StoresCredentialsAndRunsFirstSync(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()
	connector := &fakeConnector{accounts: []entities.BankSyncAccount{
		{ID: "ext-1", Label: "Compte courant", Balance: amt("50.00"), Currency: "EUR"},
	}}
	syncUC := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, connector)
	connectUC := NewConnectBankUseCase(connections, identityEncryptor{}, syncUC)

	userID := uuid.New()
	result, err := connectUC.Connect(context.Background(), userID, entities.BankSyncBankCreditMutuel, "my-login", "my-password")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(result) != 1 || !result[0].Created {
		t.Fatalf("expected the first sync to create one account, got %+v", result)
	}

	conn := connections.byUser[userID]
	if conn == nil {
		t.Fatal("expected a bank connection to have been stored")
	}
	if string(conn.LoginEncrypted) != "my-login" || string(conn.PasswordEncrypted) != "my-password" {
		t.Fatalf("expected credentials to be stored (identity-encrypted), got login=%q password=%q", conn.LoginEncrypted, conn.PasswordEncrypted)
	}
}

func TestConnectBank_AlreadyConnected_ReturnsBankSyncError(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()
	syncUC := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, &fakeConnector{})
	connectUC := NewConnectBankUseCase(connections, identityEncryptor{}, syncUC)

	userID := uuid.New()
	connections.byUser[userID] = &entities.BankConnection{ID: uuid.New(), UserID: userID}

	_, err := connectUC.Connect(context.Background(), userID, entities.BankSyncBankCreditMutuel, "login", "password")
	var syncErr *BankSyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected a *BankSyncError, got %v (%T)", err, err)
	}
}

func TestConnectBank_UnsupportedBank_ReturnsBankSyncError(t *testing.T) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeCheckpointRepo()
	operations := newFakeOperationRepo()
	syncUC := newTestSyncUseCase(t, connections, accounts, checkpoints, operations, &fakeConnector{})
	connectUC := NewConnectBankUseCase(connections, identityEncryptor{}, syncUC)

	userID := uuid.New()
	_, err := connectUC.Connect(context.Background(), userID, entities.BankSyncBank("some-other-bank"), "login", "password")
	var syncErr *BankSyncError
	if !errors.As(err, &syncErr) {
		t.Fatalf("expected a *BankSyncError, got %v (%T)", err, err)
	}
	if connections.byUser[userID] != nil {
		t.Fatal("expected no connection to be stored for an unsupported bank")
	}
}
