package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestBankProfileHandler_Create_Success(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()
	amountCol := "amount"

	body := bankProfileRequest{
		Name: "My Bank", Delimiter: ";", Encoding: "utf-8", DateFormat: "2006-01-02",
		DecimalSeparator: ",", AmountMode: "single",
		ColumnMapping: columnMappingRequest{Date: "date", Label: "label", Amount: &amountCol},
	}
	req := asUser(jsonRequest(t, "POST", "/api/bank-profiles", body, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankProfileResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Global {
		t.Error("a user-created profile should not be global")
	}
	if len(profiles.byID) != 1 {
		t.Fatalf("got %d profiles stored, want 1", len(profiles.byID))
	}
}

func TestBankProfileHandler_Create_RequiresAmountColumnForSingleMode(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()

	body := bankProfileRequest{
		Name: "My Bank", DateFormat: "2006-01-02", AmountMode: "single",
		ColumnMapping: columnMappingRequest{Date: "date", Label: "label"},
	}
	req := asUser(jsonRequest(t, "POST", "/api/bank-profiles", body, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (amount column required), body=%s", rec.Code, rec.Body)
	}
}

func TestBankProfileHandler_Create_RejectsUnknownAmountMode(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()

	amount := "amount"
	body := bankProfileRequest{
		Name: "My Bank", DateFormat: "2006-01-02", AmountMode: "signed",
		ColumnMapping: columnMappingRequest{Date: "date", Label: "label", Amount: &amount},
	}
	req := asUser(jsonRequest(t, "POST", "/api/bank-profiles", body, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (unknown amount_mode), body=%s", rec.Code, rec.Body)
	}
}

func TestBankProfileHandler_Create_RequiresDebitCreditColumnsForDebitCreditMode(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()

	body := bankProfileRequest{
		Name: "My Bank", DateFormat: "2006-01-02", AmountMode: "debit_credit",
		ColumnMapping: columnMappingRequest{Date: "date", Label: "label"},
	}
	req := asUser(jsonRequest(t, "POST", "/api/bank-profiles", body, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (debit/credit columns required), body=%s", rec.Code, rec.Body)
	}
}

func TestBankProfileHandler_List_IncludesGlobalAndOwn(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()
	global := &entities.BankProfile{ID: uuid.New(), Name: "Generic CSV"}
	mine := &entities.BankProfile{ID: uuid.New(), UserID: &user.ID, Name: "Mine"}
	profiles.byID[global.ID] = global
	profiles.byID[mine.ID] = mine

	req := asUser(jsonRequest(t, "GET", "/api/bank-profiles", nil, nil), user)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []bankProfileResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 2 {
		t.Fatalf("got %d profiles, want 2", len(resp))
	}
}

func TestBankProfileHandler_Update_NotOwner(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	owner := newTestUser()
	stranger := newTestUser()
	mine := &entities.BankProfile{ID: uuid.New(), UserID: &owner.ID, Name: "Mine", AmountMode: entities.AmountModeSingle}
	profiles.byID[mine.ID] = mine

	amountCol := "amount"
	body := bankProfileRequest{Name: "Renamed", AmountMode: "single", ColumnMapping: columnMappingRequest{Amount: &amountCol}}
	req := asUser(jsonRequest(t, "PUT", "/api/bank-profiles/"+mine.ID.String(), body, map[string]string{"profileID": mine.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestBankProfileHandler_Update_GlobalProfileNotOwned(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()
	global := &entities.BankProfile{ID: uuid.New(), Name: "Generic CSV", AmountMode: entities.AmountModeSingle}
	profiles.byID[global.ID] = global

	amountCol := "amount"
	body := bankProfileRequest{Name: "Renamed", AmountMode: "single", ColumnMapping: columnMappingRequest{Amount: &amountCol}}
	req := asUser(jsonRequest(t, "PUT", "/api/bank-profiles/"+global.ID.String(), body, map[string]string{"profileID": global.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (a global profile has no owner to authorize the edit), body=%s", rec.Code, rec.Body)
	}
}

func TestBankProfileHandler_Update_Success(t *testing.T) {
	profiles := newFakeBankProfileRepo()
	h := NewBankProfileHandler(profiles)
	user := newTestUser()
	mine := &entities.BankProfile{ID: uuid.New(), UserID: &user.ID, Name: "Old Name", AmountMode: entities.AmountModeSingle}
	profiles.byID[mine.ID] = mine

	amountCol := "amount"
	body := bankProfileRequest{Name: "New Name", AmountMode: "single", ColumnMapping: columnMappingRequest{Amount: &amountCol}}
	req := asUser(jsonRequest(t, "PUT", "/api/bank-profiles/"+mine.ID.String(), body, map[string]string{"profileID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp bankProfileResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Name != "New Name" {
		t.Errorf("Name = %q, want New Name", resp.Name)
	}
}
