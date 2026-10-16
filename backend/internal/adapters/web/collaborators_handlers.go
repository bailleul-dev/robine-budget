package web

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

type CollaboratorHandler struct {
	collaborators ports.CollaboratorRepository
	users         ports.UserRepository
	sharing       *services.SharingUseCase
}

func NewCollaboratorHandler(collaborators ports.CollaboratorRepository, users ports.UserRepository, sharing *services.SharingUseCase) *CollaboratorHandler {
	return &CollaboratorHandler{collaborators: collaborators, users: users, sharing: sharing}
}

func collaboratorStatus(c entities.Collaborator) string {
	switch {
	case c.RevokedAt != nil:
		return "revoked"
	case c.AcceptedAt != nil:
		return "accepted"
	default:
		return "pending"
	}
}

type collaboratorResponse struct {
	ID                string     `json:"id"`
	Token             string     `json:"token"`
	Status            string     `json:"status"`
	InvitedEmail      string     `json:"invited_email"`
	CollaboratorName  *string    `json:"collaborator_name"`
	CollaboratorEmail *string    `json:"collaborator_email"`
	CreatedAt         time.Time  `json:"created_at"`
	AcceptedAt        *time.Time `json:"accepted_at"`
}

func (h *CollaboratorHandler) toCollaboratorResponse(ctx context.Context, c entities.Collaborator) collaboratorResponse {
	resp := collaboratorResponse{
		ID: c.ID.String(), Token: c.Token, Status: collaboratorStatus(c), InvitedEmail: c.InvitedEmail,
		CreatedAt: c.CreatedAt, AcceptedAt: c.AcceptedAt,
	}
	if c.CollaboratorUserID != nil {
		if user, err := h.users.GetByID(ctx, *c.CollaboratorUserID); err == nil && user != nil {
			resp.CollaboratorName = &user.DisplayName
			resp.CollaboratorEmail = user.Email
		}
	}
	return resp
}

// List returns every invite the current user has issued — pending,
// accepted, or revoked — so they can see who has access and revoke it.
func (h *CollaboratorHandler) List(w http.ResponseWriter, r *http.Request) {
	collaborators, err := h.collaborators.ListForOwner(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]collaboratorResponse, len(collaborators))
	for i, c := range collaborators {
		out[i] = h.toCollaboratorResponse(r.Context(), c)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createCollaboratorRequest struct {
	Email string `json:"email"`
}

// Create issues a fresh share link for the current user's accounts,
// redeemable only by whoever signs in with the given email address.
func (h *CollaboratorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createCollaboratorRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	email := strings.TrimSpace(body.Email)
	addr, err := mail.ParseAddress(email)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid email address"))
		return
	}
	currentUser := CurrentUser(r)
	if currentUser.Email != nil && strings.EqualFold(strings.TrimSpace(*currentUser.Email), addr.Address) {
		httpx.WriteError(w, httpx.BadRequest("you can't invite yourself"))
		return
	}
	invite, err := h.sharing.CreateInvite(r.Context(), currentUser.ID, addr.Address)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.toCollaboratorResponse(r.Context(), *invite))
}

// Revoke ends one invite's access immediately, whether it's still pending
// or already accepted.
func (h *CollaboratorHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "collaboratorID")
	if err != nil {
		httpx.WriteError(w, httpx.NotFound("invite not found"))
		return
	}
	invite, err := h.collaborators.GetByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if invite == nil || invite.OwnerUserID != CurrentUser(r).ID {
		httpx.WriteError(w, httpx.NotFound("invite not found"))
		return
	}
	revoked, err := h.sharing.Revoke(r.Context(), invite)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.toCollaboratorResponse(r.Context(), *revoked))
}

type inviteInfoResponse struct {
	OwnerDisplayName string `json:"owner_display_name"`
	Status           string `json:"status"`
}

// GetInvite is public (no session required) — it backs the invite landing
// page a visitor sees before signing in, so it deliberately reveals nothing
// beyond the owner's display name and the link's status.
func (h *CollaboratorHandler) GetInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invite, err := h.collaborators.GetByToken(r.Context(), token)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if invite == nil {
		httpx.WriteError(w, httpx.NotFound("invite not found"))
		return
	}
	owner, err := h.users.GetByID(r.Context(), invite.OwnerUserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	ownerName := ""
	if owner != nil {
		ownerName = owner.DisplayName
	}
	httpx.WriteJSON(w, http.StatusOK, inviteInfoResponse{OwnerDisplayName: ownerName, Status: collaboratorStatus(*invite)})
}

type redeemInviteResponse struct {
	OwnerDisplayName string `json:"owner_display_name"`
}

// RedeemInvite grants the current (now authenticated) user access to the
// invite's owner's accounts.
func (h *CollaboratorHandler) RedeemInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	currentUser := CurrentUser(r)
	// An invite is bound to an email address, so only an address the user has
	// proven they own can claim it. Otherwise anyone could register with the
	// invited person's address and take the shared access meant for them.
	if !currentUser.EmailVerified() {
		httpx.WriteError(w, httpx.Forbidden("verify your email address before accepting an invitation"))
		return
	}
	redeemed, err := h.sharing.Redeem(r.Context(), token, currentUser.ID, currentUser.Email)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInviteNotFound):
			httpx.WriteError(w, httpx.NotFound(err.Error()))
		case errors.Is(err, services.ErrInviteRevoked), errors.Is(err, services.ErrInviteOwnInvite),
			errors.Is(err, services.ErrInviteAlreadyClaimed), errors.Is(err, services.ErrInviteEmailMismatch):
			httpx.WriteError(w, httpx.BadRequest(err.Error()))
		default:
			httpx.WriteError(w, err)
		}
		return
	}
	owner, err := h.users.GetByID(r.Context(), redeemed.OwnerUserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	ownerName := ""
	if owner != nil {
		ownerName = owner.DisplayName
	}
	httpx.WriteJSON(w, http.StatusOK, redeemInviteResponse{OwnerDisplayName: ownerName})
}
