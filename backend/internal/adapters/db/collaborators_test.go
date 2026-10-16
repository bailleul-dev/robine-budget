package db

import (
	"context"
	"testing"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
)

func TestCollaboratorRepository_AddAndGetByID(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	owner := createTestUser(t, pool)

	created, err := NewCollaboratorRepository(pool).Add(ctx, &entities.Collaborator{
		ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "invitee@example.com",
		Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", created.ID) })

	got, err := NewCollaboratorRepository(pool).GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.InvitedEmail != "invitee@example.com" {
		t.Fatalf("got %v, unexpected", got)
	}
	if got.CollaboratorUserID != nil {
		t.Errorf("CollaboratorUserID = %v, want nil before redeeming", got.CollaboratorUserID)
	}
}

func TestCollaboratorRepository_GetByToken(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	owner := createTestUser(t, pool)
	token := "token-" + newTestUUIDString()

	created, err := NewCollaboratorRepository(pool).Add(ctx, &entities.Collaborator{
		ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "invitee@example.com",
		Role: entities.CollaboratorRoleCollaborator, Token: token,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", created.ID) })

	got, err := NewCollaboratorRepository(pool).GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("got %v, want %s", got, created.ID)
	}

	miss, err := NewCollaboratorRepository(pool).GetByToken(ctx, "no-such-token")
	if err != nil {
		t.Fatalf("GetByToken (miss): %v", err)
	}
	if miss != nil {
		t.Fatalf("got %v, want nil for an unknown token", miss)
	}
}

func TestCollaboratorRepository_ListForOwner(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewCollaboratorRepository(pool)
	owner := createTestUser(t, pool)
	otherOwner := createTestUser(t, pool)

	mine, err := repo.Add(ctx, &entities.Collaborator{ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "a@example.com", Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", mine.ID) })
	theirs, err := repo.Add(ctx, &entities.Collaborator{ID: newTestUUID(), OwnerUserID: otherOwner.ID, InvitedEmail: "b@example.com", Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", theirs.ID) })

	list, err := repo.ListForOwner(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListForOwner: %v", err)
	}
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("got %v, want only %s's own invite", list, owner.ID)
	}
}

func TestCollaboratorRepository_ListActiveForCollaborator(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	repo := NewCollaboratorRepository(pool)
	owner := createTestUser(t, pool)
	collaborator := createTestUser(t, pool)
	now := time.Now()

	pending, err := repo.Add(ctx, &entities.Collaborator{ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "pending@example.com", Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add pending: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", pending.ID) })

	accepted, err := repo.Add(ctx, &entities.Collaborator{ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "active@example.com", Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add accepted: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", accepted.ID) })
	accepted.CollaboratorUserID = &collaborator.ID
	accepted.AcceptedAt = &now
	if _, err := repo.Update(ctx, accepted); err != nil {
		t.Fatalf("Update accepted: %v", err)
	}

	revoked, err := repo.Add(ctx, &entities.Collaborator{ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "revoked@example.com", Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString()})
	if err != nil {
		t.Fatalf("Add revoked: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", revoked.ID) })
	revoked.CollaboratorUserID = &collaborator.ID
	revoked.AcceptedAt = &now
	revoked.RevokedAt = &now
	if _, err := repo.Update(ctx, revoked); err != nil {
		t.Fatalf("Update revoked: %v", err)
	}

	active, err := repo.ListActiveForCollaborator(ctx, collaborator.ID)
	if err != nil {
		t.Fatalf("ListActiveForCollaborator: %v", err)
	}
	if len(active) != 1 || active[0].ID != accepted.ID {
		t.Fatalf("got %v, want only the accepted-and-not-revoked grant", active)
	}
}

func TestCollaboratorRepository_Update(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	owner := createTestUser(t, pool)
	collaborator := createTestUser(t, pool)
	now := time.Now()

	created, err := NewCollaboratorRepository(pool).Add(ctx, &entities.Collaborator{
		ID: newTestUUID(), OwnerUserID: owner.ID, InvitedEmail: "invitee@example.com",
		Role: entities.CollaboratorRoleCollaborator, Token: "token-" + newTestUUIDString(),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t.Cleanup(func() { mustExec(t, pool, "DELETE FROM collaborators WHERE id = $1", created.ID) })

	created.CollaboratorUserID = &collaborator.ID
	created.AcceptedAt = &now
	updated, err := NewCollaboratorRepository(pool).Update(ctx, created)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.CollaboratorUserID == nil || *updated.CollaboratorUserID != collaborator.ID {
		t.Errorf("CollaboratorUserID = %v, want %s", updated.CollaboratorUserID, collaborator.ID)
	}
	if updated.AcceptedAt == nil {
		t.Error("AcceptedAt = nil, want set")
	}
}
