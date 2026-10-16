package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/httpx"
	"bailleul.dev/budget-server/pkg/money"
)

type PlannedItemHandler struct {
	plannedItems        ports.PlannedItemRepository
	occurrences         ports.PlannedOccurrenceRepository
	categories          ports.CategoryRepository
	generateOccurrences *services.GenerateOccurrencesUseCase
	autoMatch           *services.ReconcileUseCase
	detectRecurring     *services.RecurringDetectionUseCase
	accounts            *AccountHandler
}

func NewPlannedItemHandler(
	plannedItems ports.PlannedItemRepository,
	occurrences ports.PlannedOccurrenceRepository,
	categories ports.CategoryRepository,
	generateOccurrences *services.GenerateOccurrencesUseCase,
	autoMatch *services.ReconcileUseCase,
	detectRecurring *services.RecurringDetectionUseCase,
	accounts *AccountHandler,
) *PlannedItemHandler {
	return &PlannedItemHandler{
		plannedItems: plannedItems, occurrences: occurrences, categories: categories,
		generateOccurrences: generateOccurrences, autoMatch: autoMatch, detectRecurring: detectRecurring, accounts: accounts,
	}
}

// resolveCategoryID validates an optional category_id from a request body —
// nil clears the category, otherwise it must be a standard category, one
// owned by the current user, or one owned by accountOwnerID (so a
// collaborator can use categories the account's owner already set up).
// Mirrors OperationHandler.SetCategory's check.
func (h *PlannedItemHandler) resolveCategoryID(r *http.Request, raw *string, accountOwnerID uuid.UUID) (*uuid.UUID, error) {
	if raw == nil {
		return nil, nil
	}
	categoryID, err := uuid.Parse(*raw)
	if err != nil {
		return nil, httpx.BadRequest("invalid category_id")
	}
	category, err := h.categories.GetByID(r.Context(), categoryID)
	if err != nil {
		return nil, err
	}
	if !categoryVisibleTo(category, CurrentUser(r).ID, accountOwnerID) {
		return nil, httpx.BadRequest("category not found")
	}
	return &categoryID, nil
}

type plannedItemResponse struct {
	ID              string  `json:"id"`
	AccountID       string  `json:"account_id"`
	Kind            string  `json:"kind"`
	Label           *string `json:"label"`
	MatchMode       string  `json:"match_mode"`
	Amount          string  `json:"amount"`
	StartDate       string  `json:"start_date"`
	RecurrenceType  string  `json:"recurrence_type"`
	OccurrenceCount *int    `json:"occurrence_count"`
	IntervalWeeks   *int    `json:"interval_weeks"`
	DayOfMonth      *int    `json:"day_of_month"`
	Active          bool    `json:"active"`
	Notes           *string `json:"notes"`
	CategoryID      *string `json:"category_id"`
}

func toPlannedItemResponse(i entities.PlannedItem) plannedItemResponse {
	var categoryID *string
	if i.CategoryID != nil {
		s := i.CategoryID.String()
		categoryID = &s
	}
	return plannedItemResponse{
		ID: i.ID.String(), AccountID: i.AccountID.String(), Kind: string(i.Kind), Label: i.Label, MatchMode: string(i.MatchMode),
		Amount: i.Amount.Decimal.StringFixed(2), StartDate: i.StartDate.String(), RecurrenceType: string(i.RecurrenceType),
		OccurrenceCount: i.OccurrenceCount, IntervalWeeks: i.IntervalWeeks, DayOfMonth: i.DayOfMonth,
		Active: i.Active, Notes: i.Notes, CategoryID: categoryID,
	}
}

type occurrenceResponse struct {
	ID                    string  `json:"id"`
	PlannedItemID         string  `json:"planned_item_id"`
	DueDate               string  `json:"due_date"`
	ExpectedAmount        string  `json:"expected_amount"`
	Status                string  `json:"status"`
	ReconciledOperationID *string `json:"reconciled_operation_id"`
	// ActualAmount is only set for a category-mode item's occurrence — the
	// live sum of every operation in its category over the period (see
	// ReconcileUseCase.CategoryBudgetActuals). nil for a label-mode item,
	// which uses ReconciledOperationID/reconciled amount instead.
	ActualAmount *string `json:"actual_amount"`
}

func toOccurrenceResponse(o entities.PlannedOccurrence, actual *money.Amount) occurrenceResponse {
	var reconciledOpID *string
	if o.ReconciledOperationID != nil {
		s := o.ReconciledOperationID.String()
		reconciledOpID = &s
	}
	var actualAmount *string
	if actual != nil {
		s := actual.Decimal.StringFixed(2)
		actualAmount = &s
	}
	return occurrenceResponse{
		ID: o.ID.String(), PlannedItemID: o.PlannedItemID.String(), DueDate: o.DueDate.String(),
		ExpectedAmount: o.ExpectedAmount.Decimal.StringFixed(2), Status: string(o.Status), ReconciledOperationID: reconciledOpID,
		ActualAmount: actualAmount,
	}
}

// toOccurrenceResponses maps a batch of occurrences at once, looking up each
// one's category-budget actual (if any) from actuals — see
// ReconcileUseCase.CategoryBudgetActuals.
func toOccurrenceResponses(occurrences []entities.PlannedOccurrence, actuals map[uuid.UUID]money.Amount) []occurrenceResponse {
	out := make([]occurrenceResponse, len(occurrences))
	for i, o := range occurrences {
		var actual *money.Amount
		if a, ok := actuals[o.ID]; ok {
			actual = &a
		}
		out[i] = toOccurrenceResponse(o, actual)
	}
	return out
}

// AccessiblePlannedItem loads the item plus the account behind it, for the
// current user's own accounts or ones shared with them — viewing occurrences
// and generating them are both part of a collaborator's granted scope.
// Editing an item (see Update) requires the stricter owner-only check.
func (h *PlannedItemHandler) AccessiblePlannedItem(r *http.Request) (*entities.PlannedItem, *entities.Account, error) {
	id, err := parseUUIDParam(r, "itemID")
	if err != nil {
		return nil, nil, httpx.NotFound("planned item not found")
	}
	item, err := h.plannedItems.GetByID(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	if item == nil {
		return nil, nil, httpx.NotFound("planned item not found")
	}
	account, err := h.accounts.accounts.GetByID(r.Context(), item.AccountID)
	if err != nil {
		return nil, nil, err
	}
	if account == nil {
		return nil, nil, httpx.NotFound("planned item not found")
	}
	ok, err := h.accounts.canAccessOwner(r.Context(), CurrentUser(r).ID, account.UserID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, httpx.NotFound("planned item not found")
	}
	return item, account, nil
}

// ListAll: every planned item across the user's own and shared accounts —
// backs the global recurring-items settings screen.
func (h *PlannedItemHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	ownerIDs, err := h.accounts.ResolveOwnerIDs(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var items []entities.PlannedItem
	for _, ownerID := range ownerIDs {
		forOwner, err := h.plannedItems.ListForUser(r.Context(), ownerID)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		items = append(items, forOwner...)
	}
	out := make([]plannedItemResponse, len(items))
	for i, item := range items {
		out[i] = toPlannedItemResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *PlannedItemHandler) ListForAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	items, err := h.plannedItems.ListForAccount(r.Context(), account.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]plannedItemResponse, len(items))
	for i, item := range items {
		out[i] = toPlannedItemResponse(item)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type plannedItemRequest struct {
	AccountID       string  `json:"account_id"`
	Kind            string  `json:"kind"`
	Label           *string `json:"label"`
	MatchMode       string  `json:"match_mode"`
	Amount          string  `json:"amount"`
	StartDate       string  `json:"start_date"`
	RecurrenceType  string  `json:"recurrence_type"`
	OccurrenceCount *int    `json:"occurrence_count"`
	IntervalWeeks   *int    `json:"interval_weeks"`
	DayOfMonth      *int    `json:"day_of_month"`
	Active          bool    `json:"active"`
	Notes           *string `json:"notes"`
	CategoryID      *string `json:"category_id"`
}

// plannedItemEnums is a request's enum fields once parsed — validate hands
// it back so Create/Update never convert the raw strings themselves.
type plannedItemEnums struct {
	kind       entities.PlannedItemKind
	matchMode  entities.PlannedItemMatchMode
	recurrence entities.RecurrenceType
}

func (body plannedItemRequest) validate() (plannedItemEnums, error) {
	var enums plannedItemEnums
	var err error
	if enums.kind, err = entities.ParsePlannedItemKind(body.Kind); err != nil {
		return enums, httpx.BadRequest(`kind must be "charge" or "income"`)
	}
	// match_mode defaults to "label" when omitted — every item before this
	// field existed, and any older mobile client that hasn't picked up the
	// new field yet, is a label-mode item.
	matchMode := body.MatchMode
	if matchMode == "" {
		matchMode = string(entities.PlannedItemMatchModeLabel)
	}
	if enums.matchMode, err = entities.ParsePlannedItemMatchMode(matchMode); err != nil {
		return enums, httpx.BadRequest(`match_mode must be "label" or "category"`)
	}
	if enums.recurrence, err = entities.ParseRecurrenceType(body.RecurrenceType); err != nil {
		return enums, httpx.BadRequest(`recurrence_type must be "one_off", "perpetual", "fixed_count" or "every_x_weeks"`)
	}

	switch enums.matchMode {
	case entities.PlannedItemMatchModeLabel:
		if body.Label == nil || strings.TrimSpace(*body.Label) == "" {
			return enums, httpx.BadRequest("label is required for match_mode=label")
		}
	case entities.PlannedItemMatchModeCategory:
		if body.CategoryID == nil || strings.TrimSpace(*body.CategoryID) == "" {
			return enums, httpx.BadRequest("category_id is required for match_mode=category")
		}
	}

	switch enums.recurrence {
	case entities.RecurrenceOneOff:
	case entities.RecurrenceFixedCount:
		if body.OccurrenceCount == nil || *body.OccurrenceCount < 1 {
			return enums, httpx.BadRequest("occurrence_count is required for recurrence_type=fixed_count")
		}
		if !validDayOfMonth(body.DayOfMonth) {
			return enums, httpx.BadRequest("day_of_month (1-31) is required for recurrence_type=perpetual or fixed_count")
		}
	case entities.RecurrencePerpetual:
		if !validDayOfMonth(body.DayOfMonth) {
			return enums, httpx.BadRequest("day_of_month (1-31) is required for recurrence_type=perpetual or fixed_count")
		}
	case entities.RecurrenceEveryXWeeks:
		if body.IntervalWeeks == nil || *body.IntervalWeeks < 1 {
			return enums, httpx.BadRequest("interval_weeks is required for recurrence_type=every_x_weeks")
		}
	}
	return enums, nil
}

func validDayOfMonth(day *int) bool {
	return day != nil && *day >= 1 && *day <= 31
}

// resolvedLabel trims the request's label and returns it only for a
// label-mode item — a category-mode budget always has a nil label
// regardless of what the client sent, since its match key is CategoryID.
func (body plannedItemRequest) resolvedLabel(matchMode entities.PlannedItemMatchMode) *string {
	if matchMode != entities.PlannedItemMatchModeLabel || body.Label == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*body.Label)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (h *PlannedItemHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body plannedItemRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	enums, err := body.validate()
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	accountID, err := uuid.Parse(body.AccountID)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid account_id"))
		return
	}
	account, err := h.accounts.accounts.GetByID(r.Context(), accountID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if account == nil {
		httpx.WriteError(w, httpx.NotFound("account not found"))
		return
	}
	if ok, err := h.accounts.canAccessOwner(r.Context(), CurrentUser(r).ID, account.UserID); err != nil {
		httpx.WriteError(w, err)
		return
	} else if !ok {
		httpx.WriteError(w, httpx.NotFound("account not found"))
		return
	}
	amount, err := money.New(body.Amount)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid amount"))
		return
	}
	startDate, err := civil.Parse(body.StartDate)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid start_date"))
		return
	}
	categoryID, err := h.resolveCategoryID(r, body.CategoryID, account.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	item := &entities.PlannedItem{
		ID: uuid.New(), AccountID: accountID, Kind: enums.kind, Label: body.resolvedLabel(enums.matchMode), MatchMode: enums.matchMode,
		Amount: amount, StartDate: startDate, RecurrenceType: enums.recurrence,
		OccurrenceCount: body.OccurrenceCount, IntervalWeeks: body.IntervalWeeks, DayOfMonth: body.DayOfMonth,
		Active: body.Active, Notes: body.Notes, CategoryID: categoryID,
	}
	created, err := h.plannedItems.Add(r.Context(), item)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPlannedItemResponse(*created))
}

// Update requires the account's actual owner — not a collaborator, however
// broad their access is otherwise (see entities.CollaboratorRoleCollaborator's
// doc comment: they can create a recurring item, not edit an existing one).
func (h *PlannedItemHandler) Update(w http.ResponseWriter, r *http.Request) {
	item, account, err := h.AccessiblePlannedItem(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if account.UserID != CurrentUser(r).ID {
		httpx.WriteError(w, httpx.Forbidden("only the account owner can edit a recurring item"))
		return
	}
	var body plannedItemRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	enums, err := body.validate()
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	amount, err := money.New(body.Amount)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid amount"))
		return
	}
	startDate, err := civil.Parse(body.StartDate)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid start_date"))
		return
	}
	categoryID, err := h.resolveCategoryID(r, body.CategoryID, account.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	item.Kind = enums.kind
	item.Label = body.resolvedLabel(enums.matchMode)
	item.MatchMode = enums.matchMode
	item.Amount = amount
	item.StartDate = startDate
	item.RecurrenceType = enums.recurrence
	item.OccurrenceCount = body.OccurrenceCount
	item.IntervalWeeks = body.IntervalWeeks
	item.DayOfMonth = body.DayOfMonth
	item.Active = body.Active
	item.Notes = body.Notes
	item.CategoryID = categoryID
	saved, err := h.plannedItems.Update(r.Context(), item)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPlannedItemResponse(*saved))
}

func (h *PlannedItemHandler) GenerateOccurrences(w http.ResponseWriter, r *http.Request) {
	item, _, err := h.AccessiblePlannedItem(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	monthsAhead := 12
	if raw := r.URL.Query().Get("months_ahead"); raw != "" {
		if n, parseErr := strconv.Atoi(raw); parseErr == nil {
			monthsAhead = n
		}
	}
	if monthsAhead < 1 || monthsAhead > 60 {
		httpx.WriteError(w, httpx.BadRequest("months_ahead must be between 1 and 60"))
		return
	}
	today := civil.Today()
	windowEnd := civil.New(today.Time.Year(), today.Time.Month(), 1).AddMonths(monthsAhead)
	// Start from the item's own start_date (not "today") so occurrences
	// already due get materialized too and can be reconciled against
	// historical imported operations.
	created, err := h.generateOccurrences.Execute(r.Context(), item.ID, item.StartDate, windowEnd)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Retroactively match already-imported operations against the
	// occurrences just created — covers e.g. creating a recurring item from
	// a past operation, whose label now matches this item's. (A no-op for a
	// category-mode item, which has no label to match by.)
	if _, err := h.autoMatch.AutoMatchByLabel(r.Context(), item.AccountID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	actuals, err := h.autoMatch.CategoryBudgetActuals(r.Context(), item.AccountID, created)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOccurrenceResponses(created, actuals))
}

func (h *PlannedItemHandler) ListOccurrences(w http.ResponseWriter, r *http.Request) {
	item, _, err := h.AccessiblePlannedItem(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	occurrences, err := h.occurrences.ListForItem(r.Context(), item.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	actuals, err := h.autoMatch.CategoryBudgetActuals(r.Context(), item.AccountID, occurrences)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOccurrenceResponses(occurrences, actuals))
}

type recurringSuggestionResponse struct {
	Label           string  `json:"label"`
	Kind            string  `json:"kind"`
	SuggestedAmount string  `json:"suggested_amount"`
	AnchorDate      string  `json:"anchor_date"`
	RecurrenceType  string  `json:"recurrence_type"`
	IntervalWeeks   *int    `json:"interval_weeks"`
	DayOfMonth      *int    `json:"day_of_month"`
	CategoryID      *string `json:"category_id"`
	OccurrenceCount int     `json:"occurrence_count"`
	LastOperationID string  `json:"last_operation_id"`
}

func toRecurringSuggestionResponse(s services.RecurringSuggestion) recurringSuggestionResponse {
	var categoryID *string
	if s.CategoryID != nil {
		id := s.CategoryID.String()
		categoryID = &id
	}
	return recurringSuggestionResponse{
		Label: s.Label, Kind: string(s.Kind), SuggestedAmount: s.SuggestedAmount.Decimal.StringFixed(2),
		AnchorDate: s.AnchorDate.String(), RecurrenceType: string(s.RecurrenceType),
		IntervalWeeks: s.IntervalWeeks, DayOfMonth: s.DayOfMonth, CategoryID: categoryID,
		OccurrenceCount: s.OccurrenceCount, LastOperationID: s.LastOperationID.String(),
	}
}

// RecurringSuggestions: labels in this account's operation history that
// repeat on a regular weekly or monthly schedule but aren't tracked by any
// PlannedItem yet — purely advisory, the user turns one into an actual item
// via the regular Create endpoint (see RecurringDetectionUseCase).
func (h *PlannedItemHandler) RecurringSuggestions(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	suggestions, err := h.detectRecurring.Detect(r.Context(), account.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]recurringSuggestionResponse, len(suggestions))
	for i, s := range suggestions {
		out[i] = toRecurringSuggestionResponse(s)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *PlannedItemHandler) ListOccurrencesForAccount(w http.ResponseWriter, r *http.Request) {
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
	occurrences, err := h.occurrences.ListForAccount(r.Context(), account.ID, dateFrom, dateTo)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	actuals, err := h.autoMatch.CategoryBudgetActuals(r.Context(), account.ID, occurrences)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOccurrenceResponses(occurrences, actuals))
}
