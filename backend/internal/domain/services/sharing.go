// Use-case: give someone else standing access to all of a user's accounts
// via a share link, without them needing an existing account on the app —
// they just sign in (typically with Google) and redeem the invite token.
// Mirrors the shape of ReconcileUseCase: a small set of state transitions
// over one repository, no HTTP concerns.
package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
)

var (
	ErrInviteNotFound       = errors.New("invite not found")
	ErrInviteRevoked        = errors.New("this invite has been revoked")
	ErrInviteOwnInvite      = errors.New("you can't redeem your own invite")
	ErrInviteAlreadyClaimed = errors.New("this invite has already been used")
	// ErrInviteEmailMismatch is deliberately generic — it must never repeat
	// the invited address back, or a link forwarded to (or intercepted by)
	// the wrong person would leak who it was actually meant for.
	ErrInviteEmailMismatch = errors.New("this invite is for a different email address than the one you're signed in with")
)

// normalizeEmail matches how an invited address is compared against a
// signed-in user's own — trimmed and lowercased, since email addresses are
// conventionally treated case-insensitively (including by Google).
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

type SharingUseCase struct {
	collaborators ports.CollaboratorRepository
	clock         ports.Clock
}

func NewSharingUseCase(collaborators ports.CollaboratorRepository, clock ports.Clock) *SharingUseCase {
	return &SharingUseCase{collaborators: collaborators, clock: clock}
}

func generateInviteToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateInvite issues a fresh share link for ownerUserID, redeemable only
// by a user whose own account email matches invitedEmail — confidentiality
// requires the link alone to not be enough. Calling it again (even for the
// same address) issues another one rather than reusing an existing invite —
// ListForOwner shows every link issued so far (pending, accepted, or
// revoked) so the owner can revoke any of them individually, which keeps
// the model simple (one row per link, no shared mutable "the" link) while
// still supporting inviting more than one person over time.
func (uc *SharingUseCase) CreateInvite(ctx context.Context, ownerUserID uuid.UUID, invitedEmail string) (*entities.Collaborator, error) {
	token, err := generateInviteToken()
	if err != nil {
		return nil, err
	}
	return uc.collaborators.Add(ctx, &entities.Collaborator{
		ID:           uuid.New(),
		OwnerUserID:  ownerUserID,
		InvitedEmail: normalizeEmail(invitedEmail),
		Role:         entities.CollaboratorRoleCollaborator,
		Token:        token,
	})
}

// Redeem grants collaboratorUserID access to ownerUserID's accounts, but
// only when collaboratorEmail matches the address the invite was issued
// for — a nil email (an account with none on file) can never match.
// Idempotent when the same user redeems the same still-active invite twice
// (revisiting the link), so returning to it later never errors out someone
// who already accepted.
func (uc *SharingUseCase) Redeem(ctx context.Context, token string, collaboratorUserID uuid.UUID, collaboratorEmail *string) (*entities.Collaborator, error) {
	invite, err := uc.collaborators.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, ErrInviteNotFound
	}
	if invite.RevokedAt != nil {
		return nil, ErrInviteRevoked
	}
	if invite.OwnerUserID == collaboratorUserID {
		return nil, ErrInviteOwnInvite
	}
	if invite.AcceptedAt != nil {
		if invite.CollaboratorUserID != nil && *invite.CollaboratorUserID == collaboratorUserID {
			return invite, nil
		}
		return nil, ErrInviteAlreadyClaimed
	}
	if collaboratorEmail == nil || normalizeEmail(*collaboratorEmail) != invite.InvitedEmail {
		return nil, ErrInviteEmailMismatch
	}

	now := uc.clock.Now()
	invite.CollaboratorUserID = &collaboratorUserID
	invite.AcceptedAt = &now
	return uc.collaborators.Update(ctx, invite)
}

// Revoke immediately drops the grant this invite represents, whether it was
// already accepted or is still pending (in which case the link simply stops
// working for anyone who tries it afterward).
func (uc *SharingUseCase) Revoke(ctx context.Context, invite *entities.Collaborator) (*entities.Collaborator, error) {
	now := uc.clock.Now()
	invite.RevokedAt = &now
	return uc.collaborators.Update(ctx, invite)
}
