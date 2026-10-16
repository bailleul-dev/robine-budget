package web

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func newTestCategoryHandler() (*CategoryHandler, *fakeCategoryRepo, *AccountHandler, *fakeAccountRepo, *fakeCollaboratorRepo) {
	categories := newFakeCategoryRepo()
	accountHandler, accounts, _, collaborators := newTestAccountHandler()
	return NewCategoryHandler(categories, accountHandler), categories, accountHandler, accounts, collaborators
}

func TestCategoryHandler_List_IncludesStandardAndOwn(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	standard := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, Name: "My hobby"}
	categories.byID[standard.ID] = standard
	categories.byID[mine.ID] = mine

	req := asUser(jsonRequest(t, "GET", "/api/categories", nil, nil), user)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []categoryResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 2 {
		t.Fatalf("got %d categories, want 2", len(resp))
	}
}

func TestCategoryHandler_Create_RequiresParent(t *testing.T) {
	h, _, _, _, _ := newTestCategoryHandler()
	user := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/categories", categoryRequest{Name: "New"}, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (parent_id is required), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Create_Success(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	parent := &entities.Category{ID: uuid.New(), Name: "Entertainment"}
	categories.byID[parent.ID] = parent

	req := asUser(jsonRequest(t, "POST", "/api/categories", categoryRequest{Name: "Streaming", ParentID: parent.ID.String()}, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp categoryResponse
	decodeJSONBody(t, rec, &resp)
	if !resp.Editable {
		t.Error("a category the viewer just created should be editable by them")
	}
	if resp.IsStandard {
		t.Error("a user-created category should not be standard")
	}
}

func TestCategoryHandler_Create_RejectsUnknownParent(t *testing.T) {
	h, _, _, _, _ := newTestCategoryHandler()
	user := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/categories", categoryRequest{Name: "New", ParentID: uuid.NewString()}, nil), user)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (unknown parent), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Update_NotOwner(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	owner := newTestUser()
	stranger := newTestUser()
	mine := &entities.Category{ID: uuid.New(), UserID: &owner.ID, Name: "Mine"}
	categories.byID[mine.ID] = mine

	req := asUser(jsonRequest(t, "PUT", "/api/categories/"+mine.ID.String(), categoryRequest{Name: "Renamed", ParentID: uuid.NewString()}, map[string]string{"categoryID": mine.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (not the owner), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Update_StandardCategoryNotEditable(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	standard := &entities.Category{ID: uuid.New(), Name: "Groceries"}
	categories.byID[standard.ID] = standard

	req := asUser(jsonRequest(t, "PUT", "/api/categories/"+standard.ID.String(), categoryRequest{Name: "Renamed", ParentID: uuid.NewString()}, map[string]string{"categoryID": standard.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404 (a standard category has no owner to authorize the edit), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Update_Success(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	oldParent := &entities.Category{ID: uuid.New(), Name: "Old Parent"}
	newParent := &entities.Category{ID: uuid.New(), Name: "New Parent"}
	categories.byID[oldParent.ID] = oldParent
	categories.byID[newParent.ID] = newParent
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, ParentID: &oldParent.ID, Name: "Old Name"}
	categories.byID[mine.ID] = mine

	body := categoryRequest{Name: "New Name", ParentID: newParent.ID.String()}
	req := asUser(jsonRequest(t, "PUT", "/api/categories/"+mine.ID.String(), body, map[string]string{"categoryID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp categoryResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Name != "New Name" {
		t.Errorf("Name = %q, want New Name", resp.Name)
	}
	if resp.ParentID == nil || *resp.ParentID != newParent.ID.String() {
		t.Errorf("ParentID = %v, want %s", resp.ParentID, newParent.ID)
	}
}

func TestCategoryHandler_Update_RejectsUnknownParent(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, Name: "Mine"}
	categories.byID[mine.ID] = mine

	body := categoryRequest{Name: "Renamed", ParentID: uuid.NewString()}
	req := asUser(jsonRequest(t, "PUT", "/api/categories/"+mine.ID.String(), body, map[string]string{"categoryID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (unknown parent), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Update_RequiresParentID(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, Name: "Mine"}
	categories.byID[mine.ID] = mine

	body := categoryRequest{Name: "Renamed"}
	req := asUser(jsonRequest(t, "PUT", "/api/categories/"+mine.ID.String(), body, map[string]string{"categoryID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Update(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (parent_id is required), body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Delete_Success(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, Name: "Mine"}
	categories.byID[mine.ID] = mine

	req := asUser(jsonRequest(t, "DELETE", "/api/categories/"+mine.ID.String(), nil, map[string]string{"categoryID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 204 {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body)
	}
}

func TestCategoryHandler_Delete_BlockedBySubcategories(t *testing.T) {
	h, categories, _, _, _ := newTestCategoryHandler()
	user := newTestUser()
	mine := &entities.Category{ID: uuid.New(), UserID: &user.ID, Name: "Mine"}
	categories.byID[mine.ID] = mine
	categories.deleteErr = &pgconn.PgError{Code: "23503"}

	req := asUser(jsonRequest(t, "DELETE", "/api/categories/"+mine.ID.String(), nil, map[string]string{"categoryID": mine.ID.String()}), user)
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (foreign key violation should surface as a clear error), body=%s", rec.Code, rec.Body)
	}
}
