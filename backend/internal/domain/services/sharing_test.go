package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

const fatherEmail = "pere@example.com"

func strPtr(s string) *string { return &s }

func TestSharing_RedeemGrantsAccess(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, err := uc.CreateInvite(context.Background(), owner, fatherEmail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	redeemed, err := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if redeemed.CollaboratorUserID == nil || *redeemed.CollaboratorUserID != father {
		t.Fatalf("expected collaborator_user_id to be set to %s, got %v", father, redeemed.CollaboratorUserID)
	}
	if redeemed.AcceptedAt == nil {
		t.Fatal("expected accepted_at to be set")
	}

	active, err := repo.ListActiveForCollaborator(context.Background(), father)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 || active[0].OwnerUserID != owner {
		t.Fatalf("expected father to have one active grant from owner, got %+v", active)
	}
}

func TestSharing_RedeemMatchesEmailCaseInsensitivelyAndTrimmed(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, err := uc.CreateInvite(context.Background(), owner, "  Pere@Example.com  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := uc.Redeem(context.Background(), invite.Token, father, strPtr("pere@example.com")); err != nil {
		t.Fatalf("expected case/whitespace-insensitive match to succeed, got %v", err)
	}
}

func TestSharing_RedeemFailsForAWrongEmail(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	intruder := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	_, err := uc.Redeem(context.Background(), invite.Token, intruder, strPtr("intrus@example.com"))
	if !errors.Is(err, ErrInviteEmailMismatch) {
		t.Fatalf("expected ErrInviteEmailMismatch, got %v", err)
	}
	// A forwarded/intercepted link must not grant access even though the
	// intruder does have a valid, distinct account.
	active, err := repo.ListActiveForCollaborator(context.Background(), intruder)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no grant for the wrong email, got %+v", active)
	}
}

func TestSharing_RedeemFailsWithNoEmailOnAccount(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	_, err := uc.Redeem(context.Background(), invite.Token, father, nil)
	if !errors.Is(err, ErrInviteEmailMismatch) {
		t.Fatalf("expected ErrInviteEmailMismatch for a nil email, got %v", err)
	}
}

func TestSharing_RedeemIsIdempotentForSameUser(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	if _, err := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail)); err != nil {
		t.Fatalf("unexpected error on first redeem: %v", err)
	}
	if _, err := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail)); err != nil {
		t.Fatalf("expected re-redeeming by the same user to succeed, got %v", err)
	}
}

func TestSharing_RedeemByAnotherUserAfterAcceptedFails(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()
	someoneElse := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	if _, err := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := uc.Redeem(context.Background(), invite.Token, someoneElse, strPtr(fatherEmail))
	if !errors.Is(err, ErrInviteAlreadyClaimed) {
		t.Fatalf("expected ErrInviteAlreadyClaimed, got %v", err)
	}
}

func TestSharing_RedeemRevokedInviteFails(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	if _, err := uc.Revoke(context.Background(), invite); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail))
	if !errors.Is(err, ErrInviteRevoked) {
		t.Fatalf("expected ErrInviteRevoked, got %v", err)
	}
}

func TestSharing_RevokeDropsActiveGrant(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()
	father := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	redeemed, _ := uc.Redeem(context.Background(), invite.Token, father, strPtr(fatherEmail))
	if _, err := uc.Revoke(context.Background(), redeemed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	active, err := repo.ListActiveForCollaborator(context.Background(), father)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected no active grants after revoke, got %+v", active)
	}
}

func TestSharing_OwnerCannotRedeemOwnInvite(t *testing.T) {
	repo := newFakeCollaboratorRepo()
	uc := NewSharingUseCase(repo, fixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	owner := uuid.New()

	invite, _ := uc.CreateInvite(context.Background(), owner, fatherEmail)
	_, err := uc.Redeem(context.Background(), invite.Token, owner, strPtr(fatherEmail))
	if !errors.Is(err, ErrInviteOwnInvite) {
		t.Fatalf("expected ErrInviteOwnInvite, got %v", err)
	}
}
