package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newTestBankSyncHandler(connector *fakeBankConnector) (*BankSyncHandler, *fakeBankConnectionRepo, *fakeAccountRepo, *fakeOperationRepo) {
	connections := newFakeBankConnectionRepo()
	accounts := newFakeAccountRepo()
	checkpoints := newFakeBalanceCheckpointRepo()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	autoCategorize := services.NewAutoCategorizeUseCase(operations, categories)
	clock := newFakeClock(civil.Today())

	syncUC := services.NewSyncBankAccountsUseCase(connections, accounts, checkpoints, operations, connector, identityEncryptor{}, clock, autoCategorize)
	connectUC := services.NewConnectBankUseCase(connections, identityEncryptor{}, syncUC)
	statusUC := services.NewBankConnectionStatusUseCase(connections)

	return NewBankSyncHandler(connectUC, syncUC, statusUC).AllowAll(), connections, accounts, operations
}

func TestBankSyncHandler_Status_NotConnected(t *testing.T) {
	h, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/bank-sync/status", nil, nil), user)
	rec := httptest.NewRecorder()
	h.Status(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankConnectionStatusResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Connected {
		t.Error("Connected should be false with no bank connection")
	}
}

func TestBankSyncHandler_Status_Connected(t *testing.T) {
	h, connections, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()
	connections.byUser[user.ID] = &entities.BankConnection{UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel}

	req := asUser(jsonRequest(t, "GET", "/api/bank-sync/status", nil, nil), user)
	rec := httptest.NewRecorder()
	h.Status(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankConnectionStatusResponse
	decodeJSONBody(t, rec, &resp)
	if !resp.Connected {
		t.Error("Connected should be true")
	}
	if resp.Bank != "cm" {
		t.Errorf("Bank = %q, want cm", resp.Bank)
	}
}

func TestBankSyncHandler_Connect_MissingFields(t *testing.T) {
	h, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()

	body := connectBankRequest{Bank: "cm", Login: "", Password: ""}
	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user)
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (login/password required), body=%s", rec.Code, rec.Body)
	}
}

func TestBankSyncHandler_Connect_UnsupportedBank(t *testing.T) {
	h, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()

	body := connectBankRequest{Bank: "unknown-bank", Login: "user", Password: "pass"}
	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user)
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (unsupported bank), body=%s", rec.Code, rec.Body)
	}
}

func TestBankSyncHandler_Connect_Success(t *testing.T) {
	connector := &fakeBankConnector{accounts: []entities.BankSyncAccount{
		{ID: "acc-1", Label: "Compte courant", Currency: "EUR", Balance: mustAmount(t, "100.00")},
	}}
	h, connections, accounts, _ := newTestBankSyncHandler(connector)
	user := newTestUser()

	body := connectBankRequest{Bank: "cm", Login: "user", Password: "pass"}
	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user)
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankSyncResultResponse
	decodeJSONBody(t, rec, &resp)
	if resp.SyncedAccounts != 1 || resp.CreatedAccounts != 1 {
		t.Errorf("got %+v, want 1 synced/created account", resp)
	}
	if len(connections.byUser) != 1 {
		t.Fatalf("got %d connections stored, want 1", len(connections.byUser))
	}
	if len(accounts.byID) != 1 {
		t.Fatalf("got %d accounts stored, want 1", len(accounts.byID))
	}
}

func TestBankSyncHandler_Connect_AlreadyConnected(t *testing.T) {
	h, connections, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()
	connections.byUser[user.ID] = &entities.BankConnection{UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel}

	body := connectBankRequest{Bank: "cm", Login: "user", Password: "pass"}
	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user)
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (already connected), body=%s", rec.Code, rec.Body)
	}
}

func TestBankSyncHandler_Connect_ConnectorError(t *testing.T) {
	connector := &fakeBankConnector{err: errAuthFailed}
	h, _, _, _ := newTestBankSyncHandler(connector)
	user := newTestUser()

	body := connectBankRequest{Bank: "cm", Login: "user", Password: "pass"}
	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user)
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (a BankSyncError surfaces as bad request), body=%s", rec.Code, rec.Body)
	}
}

func TestBankSyncHandler_Sync_NoConnection(t *testing.T) {
	h, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	user := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/sync", nil, nil), user)
	rec := httptest.NewRecorder()
	h.Sync(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (no bank connection for this user), body=%s", rec.Code, rec.Body)
	}
}

func TestBankSyncHandler_Sync_Success(t *testing.T) {
	connector := &fakeBankConnector{accounts: []entities.BankSyncAccount{
		{ID: "acc-1", Label: "Compte courant", Currency: "EUR", Balance: mustAmount(t, "100.00")},
	}}
	h, connections, _, _ := newTestBankSyncHandler(connector)
	user := newTestUser()
	connections.byUser[user.ID] = &entities.BankConnection{
		ID: newTestUser().ID, UserID: user.ID, Bank: entities.BankSyncBankCreditMutuel,
		LoginEncrypted: []byte("user"), PasswordEncrypted: []byte("pass"),
	}

	req := asUser(jsonRequest(t, "POST", "/api/bank-sync/sync", nil, nil), user)
	rec := httptest.NewRecorder()
	h.Sync(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankSyncResultResponse
	decodeJSONBody(t, rec, &resp)
	if resp.SyncedAccounts != 1 {
		t.Errorf("SyncedAccounts = %d, want 1", resp.SyncedAccounts)
	}
}

var errAuthFailed = &testAuthError{}

type testAuthError struct{}

func (e *testAuthError) Error() string { return "auth failed for user / pass" }

// --- access policy ----------------------------------------------------------------

func newClosedBankSyncHandler() *BankSyncHandler {
	h, _, _, _ := newTestBankSyncHandler(&fakeBankConnector{})
	h.allowEver = false
	return h
}

func callConnect(t *testing.T, h *BankSyncHandler, user *entities.User) *httptest.ResponseRecorder {
	t.Helper()
	body := connectBankRequest{Bank: "cm", Login: "l", Password: "p"}
	rec := httptest.NewRecorder()
	h.Connect(rec, asUser(jsonRequest(t, "POST", "/api/bank-sync/connect", body, nil), user))
	return rec
}

func TestBankSyncHandler_ClosedByDefault(t *testing.T) {
	h := newClosedBankSyncHandler()
	verified := newTestUser()
	verified.EmailVerifiedAt = verifiedNow()

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"connect": callConnect(t, h, verified),
	} {
		if rec.Code != 403 {
			t.Errorf("%s with no allow-list: status = %d, want 403", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.Sync(rec, asUser(httptest.NewRequest("POST", "/api/bank-sync/sync", nil), verified))
	if rec.Code != 403 {
		t.Errorf("sync with no allow-list: status = %d, want 403", rec.Code)
	}
}

func TestBankSyncHandler_AllowListNeedsAVerifiedAddress(t *testing.T) {
	users := newFakeUserRepo()
	h := newClosedBankSyncHandler().WithAllowList(users, []string{"Owner@Example.com"})

	email := "owner@example.com"
	unverified := &entities.User{ID: uuid.New(), Email: &email, DisplayName: "Squatter"}
	if rec := callConnect(t, h, unverified); rec.Code != 403 {
		t.Errorf("an unverified account on an allow-listed address: status = %d, want 403", rec.Code)
	}

	owner := &entities.User{ID: uuid.New(), Email: &email, DisplayName: "Owner", EmailVerifiedAt: verifiedNow()}
	if !h.available(owner) {
		t.Error("the verified allow-listed owner must be allowed (case-insensitively)")
	}
	other := "other@example.com"
	stranger := &entities.User{ID: uuid.New(), Email: &other, DisplayName: "Other", EmailVerifiedAt: verifiedNow()}
	if h.available(stranger) {
		t.Error("a verified account that is not on the list must be refused")
	}
}

func TestBankSyncHandler_StatusReportsUnavailable(t *testing.T) {
	h := newClosedBankSyncHandler()
	rec := httptest.NewRecorder()
	h.Status(rec, asUser(httptest.NewRequest("GET", "/api/bank-sync/status", nil), newTestUser()))
	var resp bankConnectionStatusResponse
	decodeJSONBody(t, rec, &resp)
	if rec.Code != 200 || resp.Available || resp.Connected {
		t.Errorf("status = %d, resp = %+v, want 200 with available=false", rec.Code, resp)
	}
}
