package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/httpx"
)

type CategoryHandler struct {
	categories ports.CategoryRepository
	accounts   *AccountHandler
}

func NewCategoryHandler(categories ports.CategoryRepository, accounts *AccountHandler) *CategoryHandler {
	return &CategoryHandler{categories: categories, accounts: accounts}
}

type categoryResponse struct {
	ID         string  `json:"id"`
	ParentID   *string `json:"parent_id"`
	Name       string  `json:"name"`
	IsStandard bool    `json:"is_standard"`
	// Editable is true only for a category the viewer owns themselves —
	// a collaborator sees the accounts' owner's own custom categories too
	// (to categorize with them) but can't rename/delete/reparent them.
	Editable bool `json:"editable"`
}

func toCategoryResponse(c entities.Category, viewerID uuid.UUID) categoryResponse {
	var parentID *string
	if c.ParentID != nil {
		s := c.ParentID.String()
		parentID = &s
	}
	return categoryResponse{
		ID: c.ID.String(), ParentID: parentID, Name: c.Name, IsStandard: c.UserID == nil,
		Editable: c.UserID != nil && *c.UserID == viewerID,
	}
}

// OwnedCategory loads a category the current user may modify — their own
// custom one. A standard category 404s here too: it exists but isn't owned
// by anyone, so it's never a valid mutation target.
func (h *CategoryHandler) OwnedCategory(r *http.Request) (*entities.Category, error) {
	id, err := parseUUIDParam(r, "categoryID")
	if err != nil {
		return nil, httpx.NotFound("category not found")
	}
	category, err := h.categories.GetByID(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if category == nil || category.UserID == nil || *category.UserID != CurrentUser(r).ID {
		return nil, httpx.NotFound("category not found")
	}
	return category, nil
}

func (h *CategoryHandler) validateParent(r *http.Request, parentID uuid.UUID) error {
	parent, err := h.categories.GetByID(r.Context(), parentID)
	if err != nil {
		return err
	}
	if parent == nil || (parent.UserID != nil && *parent.UserID != CurrentUser(r).ID) {
		return httpx.BadRequest("parent category not found")
	}
	return nil
}

// List returns every category visible to the current user: standard ones,
// their own, and — for a collaborator — the custom categories of every
// account owner who's shared with them, so they can categorize using
// categories the owner already set up instead of just standard ones.
func (h *CategoryHandler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.listVisible(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *CategoryHandler) listVisible(ctx context.Context, viewerID uuid.UUID) ([]categoryResponse, error) {
	ownerIDs, err := h.accounts.ResolveOwnerIDs(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]bool)
	var categories []entities.Category
	for _, ownerID := range ownerIDs {
		forOwner, err := h.categories.ListVisibleTo(ctx, ownerID)
		if err != nil {
			return nil, err
		}
		for _, c := range forOwner {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			categories = append(categories, c)
		}
	}
	out := make([]categoryResponse, len(categories))
	for i, c := range categories {
		out[i] = toCategoryResponse(c, viewerID)
	}
	return out, nil
}

type categoryRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

func (h *CategoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body categoryRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	parentID, err := uuid.Parse(body.ParentID)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("parent_id is required"))
		return
	}
	if err := h.validateParent(r, parentID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	userID := CurrentUser(r).ID
	category := &entities.Category{ID: uuid.New(), UserID: &userID, ParentID: &parentID, Name: body.Name}
	created, err := h.categories.Add(r.Context(), category)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCategoryResponse(*created, userID))
}

func (h *CategoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	category, err := h.OwnedCategory(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body categoryRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	parentID, err := uuid.Parse(body.ParentID)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("parent_id is required"))
		return
	}
	if err := h.validateParent(r, parentID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	category.ParentID = &parentID
	category.Name = body.Name
	saved, err := h.categories.Update(r.Context(), category)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCategoryResponse(*saved, CurrentUser(r).ID))
}

func (h *CategoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	category, err := h.OwnedCategory(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.categories.Delete(r.Context(), category.ID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
			httpx.WriteError(w, httpx.BadRequest("category has subcategories — delete or reparent them first"))
			return
		}
		httpx.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
