package web

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newTestImportHandler() (*ImportHandler, *fakeBankProfileRepo, *fakeOperationRepo, *fakeAccountRepo) {
	profiles := newFakeBankProfileRepo()
	operations := newFakeOperationRepo()
	batches := newFakeImportBatchRepo()
	categories := newFakeCategoryRepo()
	occurrences := newFakeOccurrenceRepo()
	plannedItems := newFakePlannedItemRepo()
	clock := newFakeClock(civil.Today())

	importCsv := services.NewImportCsvUseCase(operations, batches, clock)
	autoMatch := services.NewReconcileUseCase(operations, occurrences, plannedItems)
	autoCategorize := services.NewAutoCategorizeUseCase(operations, categories)
	accountHandler, accounts, _, _ := newTestAccountHandler()

	return NewImportHandler(profiles, importCsv, autoMatch, autoCategorize, accountHandler), profiles, operations, accounts
}

func TestImportHandler_Import_Success(t *testing.T) {
	h, profiles, operations, accounts := newTestImportHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	amountCol := "amount"
	profile := &entities.BankProfile{
		ID: uuid.New(), UserID: &owner.ID, Name: "Simple", Delimiter: ",", DateFormat: "%Y-%m-%d",
		DecimalSeparator: ".", AmountMode: entities.AmountModeSingle,
		ColumnMapping: entities.ColumnMapping{Date: "date", Label: "label", Amount: &amountCol},
	}
	profiles.byID[profile.ID] = profile

	csvContent := "date,label,amount\n2026-01-05,Groceries,-42.50\n"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("bank_profile_id", profile.ID.String())
	part, _ := w.CreateFormFile("file", "statement.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = w.Close()

	req := asUser(httptest.NewRequest("POST", "/api/accounts/"+account.ID.String()+"/imports", &buf), owner)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("accountID", account.ID.String())
	rec := httptest.NewRecorder()
	h.Import(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp importBatchResponse
	decodeJSONBody(t, rec, &resp)
	if resp.ImportedCount != 1 {
		t.Fatalf("ImportedCount = %d, want 1", resp.ImportedCount)
	}
	if len(operations.byID) != 1 {
		t.Fatalf("got %d operations stored, want 1", len(operations.byID))
	}
}

func TestImportHandler_Import_ForbiddenForCollaborator(t *testing.T) {
	h, profiles, _, accounts := newTestImportHandler()
	owner := newTestUser()
	collaborator := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	amountCol := "amount"
	profile := &entities.BankProfile{
		ID: uuid.New(), UserID: &owner.ID, Name: "Simple", DateFormat: "%Y-%m-%d", AmountMode: entities.AmountModeSingle,
		ColumnMapping: entities.ColumnMapping{Date: "date", Label: "label", Amount: &amountCol},
	}
	profiles.byID[profile.ID] = profile

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("bank_profile_id", profile.ID.String())
	part, _ := w.CreateFormFile("file", "statement.csv")
	_, _ = part.Write([]byte("date,label,amount\n2026-01-05,Groceries,-42.50\n"))
	_ = w.Close()

	req := asUser(httptest.NewRequest("POST", "/api/accounts/"+account.ID.String()+"/imports", &buf), collaborator)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("accountID", account.ID.String())
	rec := httptest.NewRecorder()
	h.Import(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (importing is owner-only), body=%s", rec.Code, rec.Body)
	}
}

func TestImportHandler_Import_UnknownProfile(t *testing.T) {
	h, _, _, accounts := newTestImportHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("bank_profile_id", uuid.NewString())
	part, _ := w.CreateFormFile("file", "statement.csv")
	_, _ = part.Write([]byte("date,label,amount\n2026-01-05,Groceries,-42.50\n"))
	_ = w.Close()

	req := asUser(httptest.NewRequest("POST", "/api/accounts/"+account.ID.String()+"/imports", &buf), owner)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("accountID", account.ID.String())
	rec := httptest.NewRecorder()
	h.Import(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestImportHandler_Import_MissingFile(t *testing.T) {
	h, profiles, _, accounts := newTestImportHandler()
	owner := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	accounts.byID[account.ID] = account
	profile := &entities.BankProfile{ID: uuid.New(), UserID: &owner.ID, Name: "Simple"}
	profiles.byID[profile.ID] = profile

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("bank_profile_id", profile.ID.String())
	_ = w.Close()

	req := asUser(httptest.NewRequest("POST", "/api/accounts/"+account.ID.String()+"/imports", &buf), owner)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetPathValue("accountID", account.ID.String())
	rec := httptest.NewRecorder()
	h.Import(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (file is required), body=%s", rec.Code, rec.Body)
	}
}
