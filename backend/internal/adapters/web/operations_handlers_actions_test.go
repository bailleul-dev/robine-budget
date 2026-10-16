package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/money"
)

type operationHandlerFixture struct {
	handler        *OperationHandler
	operations     *fakeOperationRepo
	categories     *fakeCategoryRepo
	splits         *fakeSplitRepo
	comments       *fakeCommentRepo
	accountHandler *AccountHandler
	accounts       *fakeAccountRepo
}

func newOperationHandlerFixture() *operationHandlerFixture {
	accountHandler, accounts, _, _ := newTestAccountHandler()
	operations := newFakeOperationRepo()
	categories := newFakeCategoryRepo()
	splits := newFakeSplitRepo()
	comments := newFakeCommentRepo()
	autoCategorize := services.NewAutoCategorizeUseCase(operations, categories)
	handler := NewOperationHandler(operations, categories, splits, comments, accountHandler, autoCategorize)
	return &operationHandlerFixture{
		handler: handler, operations: operations, categories: categories, splits: splits,
		comments: comments, accountHandler: accountHandler, accounts: accounts,
	}
}

func mustAmount(t *testing.T, s string) money.Amount {
	t.Helper()
	amt, err := money.New(s)
	if err != nil {
		t.Fatalf("money.New(%q): %v", s, err)
	}
	return amt
}

func TestOperationHandler_List_Success(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/operations", nil, map[string]string{"accountID": account.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp operationsPageResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp.Operations) != 1 {
		t.Fatalf("got %d operations, want 1", len(resp.Operations))
	}
	if resp.Operations[0].Label != "Groceries" {
		t.Errorf("Label = %q, want Groceries", resp.Operations[0].Label)
	}
}

func TestOperationHandler_List_UncategorizedOnly(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	categoryID := uuid.New()
	todo := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Unknown", Amount: mustAmount(t, "-5.00")}
	done := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00"), CategoryID: &categoryID}
	f.operations.byID[todo.ID] = todo
	f.operations.byID[done.ID] = done

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/operations?uncategorized=true", nil, map[string]string{"accountID": account.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp operationsPageResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp.Operations) != 1 || resp.Operations[0].ID != todo.ID.String() {
		t.Fatalf("operations = %+v, want only the uncategorized one", resp.Operations)
	}
}

func TestOperationHandler_List_AccountNotAccessible(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+uuid.NewString()+"/operations", nil, map[string]string{"accountID": uuid.NewString()}), user)
	rec := httptest.NewRecorder()
	f.handler.List(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_List_InvalidLimit(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account

	req := asUser(jsonRequest(t, "GET", "/api/accounts/"+account.ID.String()+"/operations?limit=0", nil, map[string]string{"accountID": account.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.List(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetCategory_Success(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op
	category := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	f.categories.byID[category.ID] = category

	categoryID := category.ID.String()
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/category", setCategoryRequest{CategoryID: &categoryID}, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetCategory(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp operationResponse
	decodeJSONBody(t, rec, &resp)
	if resp.CategoryID == nil || *resp.CategoryID != category.ID.String() {
		t.Errorf("CategoryID = %v, want %s", resp.CategoryID, category.ID)
	}
}

func TestOperationHandler_SetCategory_UnknownCategoryRejected(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op

	categoryID := uuid.NewString()
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/category", setCategoryRequest{CategoryID: &categoryID}, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetCategory(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetCategory_OtherUsersPrivateCategoryRejected(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	stranger := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op
	category := &entities.Category{ID: uuid.New(), UserID: &stranger.ID, Name: "Stranger's category"}
	f.categories.byID[category.ID] = category

	categoryID := category.ID.String()
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/category", setCategoryRequest{CategoryID: &categoryID}, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetCategory(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (a category owned by an unrelated user must not be assignable), body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetCategory_OperationNotAccessibleToStranger(t *testing.T) {
	f := newOperationHandlerFixture()
	owner := newTestUser()
	stranger := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: owner.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/category", setCategoryRequest{CategoryID: nil}, map[string]string{"operationID": op.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	f.handler.SetCategory(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetSplits_Success(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Supermarket", Amount: mustAmount(t, "-100.00")}
	f.operations.byID[op.ID] = op
	groceries := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	clothing := &entities.Category{ID: uuid.New(), Name: "Clothing"}
	f.categories.byID[groceries.ID] = groceries
	f.categories.byID[clothing.ID] = clothing

	body := setSplitsRequest{Splits: []setSplitRequest{
		{CategoryID: groceries.ID.String(), Amount: "20.00"},
		{CategoryID: clothing.ID.String(), Amount: "80.00"},
	}}
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/splits", body, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetSplits(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp operationResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp.Splits) != 2 {
		t.Fatalf("got %d splits, want 2", len(resp.Splits))
	}
}

func TestOperationHandler_SetSplits_RejectsSingleSplit(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Supermarket", Amount: mustAmount(t, "-100.00")}
	f.operations.byID[op.ID] = op
	category := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	f.categories.byID[category.ID] = category

	body := setSplitsRequest{Splits: []setSplitRequest{{CategoryID: category.ID.String(), Amount: "100.00"}}}
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/splits", body, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetSplits(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (a single split is rejected — use SetCategory instead), body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetSplits_RejectsAmountMismatch(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Supermarket", Amount: mustAmount(t, "-100.00")}
	f.operations.byID[op.ID] = op
	groceries := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	clothing := &entities.Category{ID: uuid.New(), Name: "Clothing"}
	f.categories.byID[groceries.ID] = groceries
	f.categories.byID[clothing.ID] = clothing

	body := setSplitsRequest{Splits: []setSplitRequest{
		{CategoryID: groceries.ID.String(), Amount: "20.00"},
		{CategoryID: clothing.ID.String(), Amount: "50.00"},
	}}
	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/splits", body, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetSplits(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (splits must sum to the operation amount), body=%s", rec.Code, rec.Body)
	}
}

func TestOperationHandler_SetSplits_EmptyClearsSplits(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Supermarket", Amount: mustAmount(t, "-100.00")}
	f.operations.byID[op.ID] = op
	f.splits.byOperation[op.ID] = []entities.OperationCategorySplit{{ID: uuid.New(), OperationID: op.ID, CategoryID: uuid.New(), Amount: mustAmount(t, "100.00")}}

	req := asUser(jsonRequest(t, "PUT", "/api/operations/"+op.ID.String()+"/splits", setSplitsRequest{}, map[string]string{"operationID": op.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.SetSplits(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp operationResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp.Splits) != 0 {
		t.Fatalf("got %d splits, want 0 after clearing", len(resp.Splits))
	}
}

func TestOperationHandler_Comments_AddAndList(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "Groceries", Amount: mustAmount(t, "-10.00")}
	f.operations.byID[op.ID] = op

	addReq := asUser(jsonRequest(t, "POST", "/api/operations/"+op.ID.String()+"/comments", createCommentRequest{Body: "worth double-checking"}, map[string]string{"operationID": op.ID.String()}), user)
	addRec := httptest.NewRecorder()
	f.handler.AddComment(addRec, addReq)
	if addRec.Code != 200 {
		t.Fatalf("AddComment status = %d, want 200, body=%s", addRec.Code, addRec.Body)
	}

	listReq := asUser(jsonRequest(t, "GET", "/api/operations/"+op.ID.String()+"/comments", nil, map[string]string{"operationID": op.ID.String()}), user)
	listRec := httptest.NewRecorder()
	f.handler.ListComments(listRec, listReq)
	if listRec.Code != 200 {
		t.Fatalf("ListComments status = %d, want 200, body=%s", listRec.Code, listRec.Body)
	}
	var resp []commentResponse
	decodeJSONBody(t, listRec, &resp)
	if len(resp) != 1 || resp[0].Body != "worth double-checking" {
		t.Fatalf("got %+v, want one comment with the posted body", resp)
	}
}

func TestOperationHandler_AutoCategorize_CategorizesKnownMerchant(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()
	account := &entities.Account{ID: uuid.New(), UserID: user.ID, Name: "Checking", Currency: "EUR"}
	f.accounts.byID[account.ID] = account
	restaurants := &entities.Category{ID: uuid.New(), Name: "Restaurants"}
	f.categories.byID[restaurants.ID] = restaurants
	// A merchant label from the built-in dictionary (see
	// services.merchantDictionary) so the static-dictionary tier matches
	// without needing any cross-user learning history set up. Matching is
	// whole-word, so this must contain "kfc" as its own token exactly.
	op := &entities.Operation{ID: uuid.New(), AccountID: account.ID, Date: civil.Today(), Label: "KFC PARIS", Amount: mustAmount(t, "-12.50")}
	f.operations.byID[op.ID] = op

	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+account.ID.String()+"/operations/auto-categorize", nil, map[string]string{"accountID": account.ID.String()}), user)
	rec := httptest.NewRecorder()
	f.handler.AutoCategorize(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp autoCategorizeResponse
	decodeJSONBody(t, rec, &resp)
	if resp.CategorizedCount != 1 {
		t.Fatalf("CategorizedCount = %d, want 1", resp.CategorizedCount)
	}
	if f.operations.byID[op.ID].CategoryID == nil {
		t.Error("operation should have been assigned a category")
	}
}

func TestOperationHandler_AutoCategorize_AccountNotAccessible(t *testing.T) {
	f := newOperationHandlerFixture()
	user := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/accounts/"+uuid.NewString()+"/operations/auto-categorize", nil, map[string]string{"accountID": uuid.NewString()}), user)
	rec := httptest.NewRecorder()
	f.handler.AutoCategorize(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}
