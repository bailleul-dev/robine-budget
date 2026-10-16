package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
)

func newTestCollaboratorHandler() (*CollaboratorHandler, *fakeCollaboratorRepo, *fakeUserRepo) {
	collaborators := newFakeCollaboratorRepo()
	users := newFakeUserRepo()
	sharing := services.NewSharingUseCase(collaborators, newFakeClock(civil.Today()))
	return NewCollaboratorHandler(collaborators, users, sharing), collaborators, users
}

func TestCollaboratorHandler_Create_Success(t *testing.T) {
	h, collaborators, users := newTestCollaboratorHandler()
	owner := newTestUser()
	users.byID[owner.ID] = owner

	req := asUser(jsonRequest(t, "POST", "/api/collaborators", createCollaboratorRequest{Email: "friend@example.com"}, nil), owner)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp collaboratorResponse
	decodeJSONBody(t, rec, &resp)
	if resp.InvitedEmail != "friend@example.com" {
		t.Errorf("InvitedEmail = %q, want friend@example.com", resp.InvitedEmail)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q, want pending", resp.Status)
	}
	if len(collaborators.byID) != 1 {
		t.Fatalf("got %d collaborators stored, want 1", len(collaborators.byID))
	}
}

func TestCollaboratorHandler_Create_RejectsSelfInvite(t *testing.T) {
	h, _, _ := newTestCollaboratorHandler()
	email := "me@example.com"
	owner := &entities.User{ID: uuid.New(), Email: &email, DisplayName: "Me"}

	req := asUser(jsonRequest(t, "POST", "/api/collaborators", createCollaboratorRequest{Email: "me@example.com"}, nil), owner)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (can't invite yourself), body=%s", rec.Code, rec.Body)
	}
}

func TestCollaboratorHandler_Create_RejectsInvalidEmail(t *testing.T) {
	h, _, _ := newTestCollaboratorHandler()
	owner := newTestUser()

	req := asUser(jsonRequest(t, "POST", "/api/collaborators", createCollaboratorRequest{Email: "not-an-email"}, nil), owner)
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body)
	}
}

func TestCollaboratorHandler_List_OnlyOwnersOwnInvites(t *testing.T) {
	h, collaborators, _ := newTestCollaboratorHandler()
	owner := newTestUser()
	otherOwner := newTestUser()
	mine := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: "a@example.com", Token: "tok-a"}
	theirs := &entities.Collaborator{ID: uuid.New(), OwnerUserID: otherOwner.ID, InvitedEmail: "b@example.com", Token: "tok-b"}
	collaborators.byID[mine.ID] = mine
	collaborators.byID[theirs.ID] = theirs

	req := asUser(jsonRequest(t, "GET", "/api/collaborators", nil, nil), owner)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp []collaboratorResponse
	decodeJSONBody(t, rec, &resp)
	if len(resp) != 1 || resp[0].ID != mine.ID.String() {
		t.Fatalf("got %+v, want only the owner's own invite", resp)
	}
}

func TestCollaboratorHandler_Revoke_NotOwner(t *testing.T) {
	h, collaborators, _ := newTestCollaboratorHandler()
	owner := newTestUser()
	stranger := newTestUser()
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: "a@example.com", Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	req := asUser(jsonRequest(t, "DELETE", "/api/collaborators/"+invite.ID.String(), nil, map[string]string{"collaboratorID": invite.ID.String()}), stranger)
	rec := httptest.NewRecorder()
	h.Revoke(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestCollaboratorHandler_Revoke_Success(t *testing.T) {
	h, collaborators, _ := newTestCollaboratorHandler()
	owner := newTestUser()
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: "a@example.com", Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	req := asUser(jsonRequest(t, "DELETE", "/api/collaborators/"+invite.ID.String(), nil, map[string]string{"collaboratorID": invite.ID.String()}), owner)
	rec := httptest.NewRecorder()
	h.Revoke(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp collaboratorResponse
	decodeJSONBody(t, rec, &resp)
	if resp.Status != "revoked" {
		t.Errorf("Status = %q, want revoked", resp.Status)
	}
}

func TestCollaboratorHandler_GetInvite_Public(t *testing.T) {
	h, collaborators, users := newTestCollaboratorHandler()
	owner := newTestUser()
	owner.DisplayName = "Alice"
	users.byID[owner.ID] = owner
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: "a@example.com", Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	req := httptest.NewRequest("GET", "/api/invites/tok-a", nil)
	req.SetPathValue("token", "tok-a")
	rec := httptest.NewRecorder()
	h.GetInvite(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp inviteInfoResponse
	decodeJSONBody(t, rec, &resp)
	if resp.OwnerDisplayName != "Alice" {
		t.Errorf("OwnerDisplayName = %q, want Alice", resp.OwnerDisplayName)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q, want pending", resp.Status)
	}
}

func TestCollaboratorHandler_GetInvite_UnknownToken(t *testing.T) {
	h, _, _ := newTestCollaboratorHandler()

	req := httptest.NewRequest("GET", "/api/invites/unknown", nil)
	req.SetPathValue("token", "unknown")
	rec := httptest.NewRecorder()
	h.GetInvite(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func TestCollaboratorHandler_RedeemInvite_Success(t *testing.T) {
	h, collaborators, users := newTestCollaboratorHandler()
	owner := newTestUser()
	owner.DisplayName = "Alice"
	users.byID[owner.ID] = owner
	inviteEmail := "friend@example.com"
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: inviteEmail, Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	redeemer := &entities.User{ID: uuid.New(), Email: &inviteEmail, DisplayName: "Friend", EmailVerifiedAt: verifiedNow()}
	req := asUser(httptest.NewRequest("POST", "/api/invites/tok-a/redeem", nil), redeemer)
	req.SetPathValue("token", "tok-a")
	rec := httptest.NewRecorder()
	h.RedeemInvite(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body)
	}
	var resp redeemInviteResponse
	decodeJSONBody(t, rec, &resp)
	if resp.OwnerDisplayName != "Alice" {
		t.Errorf("OwnerDisplayName = %q, want Alice", resp.OwnerDisplayName)
	}
	if invite.CollaboratorUserID == nil || *invite.CollaboratorUserID != redeemer.ID {
		t.Error("the invite should now be linked to the redeeming user")
	}
}

func TestCollaboratorHandler_RedeemInvite_EmailMismatch(t *testing.T) {
	h, collaborators, users := newTestCollaboratorHandler()
	owner := newTestUser()
	users.byID[owner.ID] = owner
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: "friend@example.com", Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	wrongEmail := "someone-else@example.com"
	redeemer := &entities.User{ID: uuid.New(), Email: &wrongEmail, DisplayName: "Someone Else", EmailVerifiedAt: verifiedNow()}
	req := asUser(httptest.NewRequest("POST", "/api/invites/tok-a/redeem", nil), redeemer)
	req.SetPathValue("token", "tok-a")
	rec := httptest.NewRecorder()
	h.RedeemInvite(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400 (email mismatch), body=%s", rec.Code, rec.Body)
	}
}

func TestCollaboratorHandler_RedeemInvite_UnknownToken(t *testing.T) {
	h, _, _ := newTestCollaboratorHandler()
	redeemer := newTestUser()
	redeemer.EmailVerifiedAt = verifiedNow()

	req := asUser(httptest.NewRequest("POST", "/api/invites/unknown/redeem", nil), redeemer)
	req.SetPathValue("token", "unknown")
	rec := httptest.NewRecorder()
	h.RedeemInvite(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body)
	}
}

func verifiedNow() *time.Time { t := time.Now(); return &t }

// Security regression: an invite is bound to an email address, so a user who
// merely *typed* that address at registration (never verified it) must not be
// able to claim it.
func TestCollaboratorHandler_RedeemInvite_UnverifiedEmailCannotClaim(t *testing.T) {
	h, collaborators, users := newTestCollaboratorHandler()
	owner := newTestUser()
	users.byID[owner.ID] = owner
	inviteEmail := "friend@example.com"
	invite := &entities.Collaborator{ID: uuid.New(), OwnerUserID: owner.ID, InvitedEmail: inviteEmail, Token: "tok-a"}
	collaborators.byID[invite.ID] = invite

	impostor := &entities.User{ID: uuid.New(), Email: &inviteEmail, DisplayName: "Impostor"} // same address, never verified
	req := asUser(httptest.NewRequest("POST", "/api/invites/tok-a/redeem", nil), impostor)
	req.SetPathValue("token", "tok-a")
	rec := httptest.NewRecorder()
	h.RedeemInvite(rec, req)

	if rec.Code != 403 {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body)
	}
	if invite.AcceptedAt != nil || invite.CollaboratorUserID != nil {
		t.Error("the invite was claimed by an unverified account")
	}
}
