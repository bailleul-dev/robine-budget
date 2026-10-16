package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

type exportFixture struct {
	handler     *ExportHandler
	accounts    *fakeAccountRepo
	operations  *fakeOperationRepo
	comments    *fakeCommentRepo
	collaborate *fakeCollaboratorRepo
	connections *fakeBankConnectionRepo
}

func newExportFixture() *exportFixture {
	accounts := newFakeAccountRepo()
	operations := newFakeOperationRepo()
	comments := newFakeCommentRepo()
	collaborators := newFakeCollaboratorRepo()
	connections := newFakeBankConnectionRepo()
	uc := services.NewExportUserDataUseCase(
		accounts, newFakeBalanceCheckpointRepo(), operations, newFakeSplitRepo(), newFakeCategoryRepo(),
		newFakePlannedItemRepo(), newFakeOccurrenceRepo(), newFakeBankProfileRepo(), comments, collaborators,
		newFakeImportBatchRepo(), connections,
	)
	return &exportFixture{NewExportHandler(uc), accounts, operations, comments, collaborators, connections}
}

func TestExport_ContainsOwnDataAndNoSecrets(t *testing.T) {
	f := newExportFixture()
	me := newTestUser()
	me.PasswordHash = strPtr("$2a$10$SECRETBCRYPTHASHSECRETBCRYPTHASHSECRETBCRYPTHASHSECRETBC")
	other := newTestUser()

	mine := &entities.Account{ID: uuid.New(), UserID: me.ID, Name: "Mon compte", Currency: "EUR"}
	theirs := &entities.Account{ID: uuid.New(), UserID: other.ID, Name: "Compte d'un autre", Currency: "EUR"}
	f.accounts.byID[mine.ID] = mine
	f.accounts.byID[theirs.ID] = theirs
	f.operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: mine.ID, Date: civil.Today(), Label: "Boulangerie", Amount: mustAmount(t, "-3.20")}
	f.operations.byID[uuid.New()] = &entities.Operation{ID: uuid.New(), AccountID: theirs.ID, Date: civil.Today(), Label: "Salaire de quelqu'un d'autre", Amount: mustAmount(t, "9999.00")}
	f.comments.byOperation[uuid.New()] = []entities.Comment{{ID: uuid.New(), UserID: me.ID, Body: "mon commentaire"}, {ID: uuid.New(), UserID: other.ID, Body: "commentaire d'un autre"}}
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: me.ID, InvitedEmail: "ami@example.com", Token: "SUPER-SECRET-INVITE-TOKEN", CreatedAt: me.CreatedAt}
	f.collaborate.byID[invite.ID] = invite

	rec := httptest.NewRecorder()
	f.handler.Export(rec, asUser(httptest.NewRequest("GET", "/api/auth/me/export", nil), me))
	if rec.Code != 200 {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", got)
	}
	body := rec.Body.String()

	for _, want := range []string{"Mon compte", "Boulangerie", "mon commentaire", "ami@example.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("export is missing %q", want)
		}
	}
	for _, leaked := range []string{"SECRETBCRYPT", "SUPER-SECRET-INVITE-TOKEN", "Compte d'un autre", "quelqu'un d'autre", "commentaire d'un autre", "password_hash"} {
		if strings.Contains(body, leaked) {
			t.Errorf("export leaks %q", leaked)
		}
	}
	var doc exportDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if len(doc.Accounts) != 1 || len(doc.Accounts[0].Operations) != 1 {
		t.Errorf("want exactly my 1 account with 1 operation, got %+v", doc.Accounts)
	}
	if !doc.User.HasPassword {
		t.Error("the export should say a password is set (without revealing it)")
	}
}

func TestExport_EmptyAccountHasStableShape(t *testing.T) {
	f := newExportFixture()
	rec := httptest.NewRecorder()
	f.handler.Export(rec, asUser(httptest.NewRequest("GET", "/api/auth/me/export", nil), newTestUser()))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, key := range []string{`"accounts": []`, `"categories": []`, `"comments": []`, `"collaborators": []`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Errorf("empty export should contain %s (not null)", key)
		}
	}
}

func TestExport_IsRateLimitedPerUser(t *testing.T) {
	f := newExportFixture()
	me := newTestUser()
	call := func(u *entities.User) int {
		rec := httptest.NewRecorder()
		f.handler.Export(rec, asUser(httptest.NewRequest("GET", "/api/auth/me/export", nil), u))
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if got := call(me); got != 200 {
			t.Fatalf("export %d: status = %d, want 200", i+1, got)
		}
	}
	if got := call(me); got != 429 {
		t.Errorf("4th export in a row: status = %d, want 429", got)
	}
	if got := call(newTestUser()); got != 200 {
		t.Errorf("another user must have their own budget: status = %d", got)
	}
}
