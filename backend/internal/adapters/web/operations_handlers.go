package web

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/httpx"
	"bailleul.dev/budget-server/pkg/money"
)

const (
	defaultOperationsPageLimit = 100
	maxOperationsPageLimit     = 500
)

type OperationHandler struct {
	operations     ports.OperationRepository
	categories     ports.CategoryRepository
	splits         ports.OperationCategorySplitRepository
	comments       ports.CommentRepository
	accounts       *AccountHandler
	autoCategorize *services.AutoCategorizeUseCase
}

func NewOperationHandler(
	operations ports.OperationRepository,
	categories ports.CategoryRepository,
	splits ports.OperationCategorySplitRepository,
	comments ports.CommentRepository,
	accounts *AccountHandler,
	autoCategorize *services.AutoCategorizeUseCase,
) *OperationHandler {
	return &OperationHandler{
		operations: operations, categories: categories, splits: splits, comments: comments,
		accounts: accounts, autoCategorize: autoCategorize,
	}
}

type operationSplitResponse struct {
	ID         string `json:"id"`
	CategoryID string `json:"category_id"`
	Amount     string `json:"amount"`
}

func toOperationSplitResponse(s entities.OperationCategorySplit) operationSplitResponse {
	return operationSplitResponse{ID: s.ID.String(), CategoryID: s.CategoryID.String(), Amount: s.Amount.Decimal.StringFixed(2)}
}

type operationResponse struct {
	ID           string                   `json:"id"`
	Date         string                   `json:"date"`
	Label        string                   `json:"label"`
	Amount       string                   `json:"amount"`
	BalanceAfter *string                  `json:"balance_after"`
	RawReference *string                  `json:"raw_reference"`
	CategoryID   *string                  `json:"category_id"`
	Splits       []operationSplitResponse `json:"splits"`
}

func toOperationResponse(o entities.Operation, splits []entities.OperationCategorySplit) operationResponse {
	var balanceAfter *string
	if o.BalanceAfter != nil {
		s := o.BalanceAfter.Decimal.StringFixed(2)
		balanceAfter = &s
	}
	var categoryID *string
	if o.CategoryID != nil {
		s := o.CategoryID.String()
		categoryID = &s
	}
	splitsOut := make([]operationSplitResponse, len(splits))
	for i, s := range splits {
		splitsOut[i] = toOperationSplitResponse(s)
	}
	return operationResponse{
		ID: o.ID.String(), Date: o.Date.String(), Label: o.Label, Amount: o.Amount.Decimal.StringFixed(2),
		BalanceAfter: balanceAfter, RawReference: o.RawReference, CategoryID: categoryID, Splits: splitsOut,
	}
}

type operationsPageResponse struct {
	Operations []operationResponse `json:"operations"`
	// NextCursor is nil once there's nothing more to fetch — pass it back
	// verbatim as ?cursor= to get the next page.
	NextCursor *string `json:"next_cursor"`
}

// encodeOperationCursor/decodeOperationCursor turn a page's last row's sort
// key into an opaque, URL-safe token clients pass back as ?cursor= to resume
// right after it — keyset pagination rather than OFFSET, so a page stays
// cheap and stable regardless of how deep into the account's history it is
// or how many operations get imported between fetches.
func encodeOperationCursor(c entities.OperationPageCursor) string {
	raw := c.Date.String() + "|" + c.CreatedAt.Format(time.RFC3339Nano) + "|" + c.ID.String()
	return base64.URLEncoding.EncodeToString([]byte(raw))
}

func decodeOperationCursor(raw string) (*entities.OperationPageCursor, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return nil, httpx.BadRequest("invalid cursor")
	}
	date, err := civil.Parse(parts[0])
	if err != nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		return nil, httpx.BadRequest("invalid cursor")
	}
	return &entities.OperationPageCursor{Date: date, CreatedAt: createdAt, ID: id}, nil
}

// parsePageLimit reads the optional ?limit= query param shared by
// keyset-paginated list endpoints.
func parsePageLimit(r *http.Request, def, max int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return 0, httpx.BadRequest(fmt.Sprintf("limit must be between 1 and %d", max))
	}
	return n, nil
}

// AccessibleOperation loads the operation plus the account behind it, for
// the current user's own accounts or ones shared with them — categorizing,
// splitting and commenting are all part of a collaborator's granted scope.
func (h *OperationHandler) AccessibleOperation(r *http.Request) (*entities.Operation, *entities.Account, error) {
	id, err := parseUUIDParam(r, "operationID")
	if err != nil {
		return nil, nil, httpx.NotFound("operation not found")
	}
	return h.accessibleOperationByID(r.Context(), CurrentUser(r).ID, id)
}

func (h *OperationHandler) accessibleOperationByID(ctx context.Context, viewerID, id uuid.UUID) (*entities.Operation, *entities.Account, error) {
	op, err := h.operations.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if op == nil {
		return nil, nil, httpx.NotFound("operation not found")
	}
	account, err := h.accounts.accounts.GetByID(ctx, op.AccountID)
	if err != nil {
		return nil, nil, err
	}
	if account == nil {
		return nil, nil, httpx.NotFound("operation not found")
	}
	ok, err := h.accounts.canAccessOwner(ctx, viewerID, account.UserID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, httpx.NotFound("operation not found")
	}
	return op, account, nil
}

// categoryVisibleTo checks a category can be assigned by viewerID on an
// operation belonging to an account owned by accountOwnerID — either a
// standard category, one the viewer owns themselves, or one owned by the
// account's actual owner (so a collaborator can use categories the owner
// already set up, not just their own).
func categoryVisibleTo(category *entities.Category, viewerID, accountOwnerID uuid.UUID) bool {
	if category == nil {
		return false
	}
	if category.UserID == nil {
		return true
	}
	return *category.UserID == viewerID || *category.UserID == accountOwnerID
}

// List keyset-paginates newest first — see ListPageForAccount. Pass the
// previous response's next_cursor back as ?cursor= to fetch the next page;
// omit it (or use nil) to start from the most recent operation. ?limit=
// defaults to 100, capped at 500. ?uncategorized=true keeps only the
// operations with neither a category nor splits.
func (h *OperationHandler) List(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	dateFrom, dateTo, err := parseDateRange(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	limit, err := parsePageLimit(r, defaultOperationsPageLimit, maxOperationsPageLimit)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	after, err := decodeOperationCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	uncategorizedOnly := r.URL.Query().Get("uncategorized") == "true"
	page, err := h.listPage(r.Context(), account.ID, dateFrom, dateTo, uncategorizedOnly, after, limit)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

// listPage fetches one page of an (already access-checked) account's
// operations, with their splits and the cursor to the next page.
func (h *OperationHandler) listPage(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date, uncategorizedOnly bool, after *entities.OperationPageCursor, limit int) (operationsPageResponse, error) {
	ops, err := h.operations.ListPageForAccount(ctx, accountID, dateFrom, dateTo, uncategorizedOnly, after, limit+1)
	if err != nil {
		return operationsPageResponse{}, err
	}
	hasMore := len(ops) > limit
	if hasMore {
		ops = ops[:limit]
	}

	ids := make([]uuid.UUID, len(ops))
	for i, o := range ops {
		ids[i] = o.ID
	}
	allSplits, err := h.splits.ListForOperationIDs(ctx, ids)
	if err != nil {
		return operationsPageResponse{}, err
	}
	splitsByOperation := make(map[uuid.UUID][]entities.OperationCategorySplit, len(allSplits))
	for _, s := range allSplits {
		splitsByOperation[s.OperationID] = append(splitsByOperation[s.OperationID], s)
	}

	out := make([]operationResponse, len(ops))
	for i, o := range ops {
		out[i] = toOperationResponse(o, splitsByOperation[o.ID])
	}

	var nextCursor *string
	if hasMore && len(ops) > 0 {
		last := ops[len(ops)-1]
		c := encodeOperationCursor(entities.OperationPageCursor{Date: last.Date, CreatedAt: last.CreatedAt, ID: last.ID})
		nextCursor = &c
	}
	return operationsPageResponse{Operations: out, NextCursor: nextCursor}, nil
}

type setCategoryRequest struct {
	CategoryID *string `json:"category_id"`
}

func (h *OperationHandler) SetCategory(w http.ResponseWriter, r *http.Request) {
	op, account, err := h.AccessibleOperation(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body setCategoryRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	var categoryID *uuid.UUID
	if body.CategoryID != nil {
		id, err := uuid.Parse(*body.CategoryID)
		if err != nil {
			httpx.WriteError(w, httpx.BadRequest("invalid category_id"))
			return
		}
		categoryID = &id
	}
	saved, err := h.assignCategory(r.Context(), op, account, CurrentUser(r).ID, categoryID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOperationResponse(*saved, nil))
}

// assignCategory sets (or, with a nil categoryID, clears) op's single
// category on behalf of viewerID, after checking the category is one they
// may use on account.
func (h *OperationHandler) assignCategory(ctx context.Context, op *entities.Operation, account *entities.Account, viewerID uuid.UUID, categoryID *uuid.UUID) (*entities.Operation, error) {
	if categoryID != nil {
		category, err := h.categories.GetByID(ctx, *categoryID)
		if err != nil {
			return nil, err
		}
		if !categoryVisibleTo(category, viewerID, account.UserID) {
			return nil, httpx.BadRequest("category not found")
		}
	}
	op.CategoryID = categoryID
	saved, err := h.operations.Update(ctx, op)
	if err != nil {
		return nil, err
	}
	// A plain single-category assignment supersedes any existing split —
	// the two are mutually exclusive ways of categorizing an operation.
	if _, err := h.splits.ReplaceForOperation(ctx, saved.ID, nil); err != nil {
		return nil, err
	}
	return saved, nil
}

type setSplitRequest struct {
	CategoryID string `json:"category_id"`
	Amount     string `json:"amount"`
}

type setSplitsRequest struct {
	Splits []setSplitRequest `json:"splits"`
}

// SetSplits divides an operation's amount across several categories — e.g. a
// 100€ supermarket run split into 20€ groceries + 80€ clothing — instead of
// the single category_id a plain SetCategory call assigns. Splits and
// category_id are mutually exclusive: setting one clears the other. Passing
// an empty splits array just clears the operation's splits.
func (h *OperationHandler) SetSplits(w http.ResponseWriter, r *http.Request) {
	op, account, err := h.AccessibleOperation(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body setSplitsRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if len(body.Splits) == 1 {
		httpx.WriteError(w, httpx.BadRequest("a single split isn't allowed — use the category endpoint instead"))
		return
	}

	currentUserID := CurrentUser(r).ID
	splits := make([]entities.OperationCategorySplit, len(body.Splits))
	total := money.Zero()
	for i, s := range body.Splits {
		categoryID, err := uuid.Parse(s.CategoryID)
		if err != nil {
			httpx.WriteError(w, httpx.BadRequest("invalid category_id"))
			return
		}
		category, err := h.categories.GetByID(r.Context(), categoryID)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		if !categoryVisibleTo(category, currentUserID, account.UserID) {
			httpx.WriteError(w, httpx.BadRequest("category not found"))
			return
		}
		amount, err := money.New(s.Amount)
		if err != nil || !amount.Decimal.IsPositive() {
			httpx.WriteError(w, httpx.BadRequest("split amount must be a positive number"))
			return
		}
		splits[i] = entities.OperationCategorySplit{OperationID: op.ID, CategoryID: categoryID, Amount: amount}
		total = total.Add(amount)
	}
	if len(splits) > 0 && !total.Decimal.Equal(op.Amount.Decimal.Abs()) {
		httpx.WriteError(w, httpx.BadRequest(
			"split amounts must add up to the operation's amount ("+op.Amount.Decimal.Abs().StringFixed(2)+")",
		))
		return
	}

	saved, err := h.splits.ReplaceForOperation(r.Context(), op.ID, splits)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Splits supersede any plain single-category assignment.
	if op.CategoryID != nil {
		op.CategoryID = nil
		if op, err = h.operations.Update(r.Context(), op); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, toOperationResponse(*op, saved))
}

type commentResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func toCommentResponse(c entities.Comment) commentResponse {
	return commentResponse{ID: c.ID.String(), UserID: c.UserID.String(), Body: c.Body, CreatedAt: c.CreatedAt}
}

func (h *OperationHandler) ListComments(w http.ResponseWriter, r *http.Request) {
	op, _, err := h.AccessibleOperation(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	comments, err := h.comments.ListForOperation(r.Context(), op.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]commentResponse, len(comments))
	for i, c := range comments {
		out[i] = toCommentResponse(c)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createCommentRequest struct {
	Body string `json:"body"`
}

func (h *OperationHandler) AddComment(w http.ResponseWriter, r *http.Request) {
	op, _, err := h.AccessibleOperation(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body createCommentRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	comment := &entities.Comment{ID: uuid.New(), OperationID: op.ID, UserID: CurrentUser(r).ID, Body: body.Body}
	created, err := h.comments.Add(r.Context(), comment)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCommentResponse(*created))
}

// parseDateRange reads optional date_from/date_to query params shared by
// every account-scoped list endpoint.
func parseDateRange(r *http.Request) (dateFrom, dateTo *civil.Date, err error) {
	if raw := r.URL.Query().Get("date_from"); raw != "" {
		d, parseErr := civil.Parse(raw)
		if parseErr != nil {
			return nil, nil, httpx.BadRequest("invalid date_from")
		}
		dateFrom = &d
	}
	if raw := r.URL.Query().Get("date_to"); raw != "" {
		d, parseErr := civil.Parse(raw)
		if parseErr != nil {
			return nil, nil, httpx.BadRequest("invalid date_to")
		}
		dateTo = &d
	}
	return dateFrom, dateTo, nil
}

type autoCategorizeResponse struct {
	CategorizedCount int `json:"categorized_count"`
}

// AutoCategorize assigns a category to every currently-uncategorized
// operation in the account it can (see services.AutoCategorizeUseCase) —
// same access level as List/SetCategory (a collaborator can trigger it on
// a shared account, not just the owner), since it only ever fills in a
// category, same as anything else category-related a collaborator can
// already do one operation at a time.
func (h *OperationHandler) AutoCategorize(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	count, err := h.autoCategorize.CategorizeAccount(r.Context(), account.ID, account.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, autoCategorizeResponse{CategorizedCount: count})
}
