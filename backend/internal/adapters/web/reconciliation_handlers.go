package web

import (
	"github.com/google/uuid"
	"net/http"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/httpx"
)

type ReconciliationHandler struct {
	operations   ports.OperationRepository
	occurrences  ports.PlannedOccurrenceRepository
	plannedItems ports.PlannedItemRepository
	accounts     *AccountHandler
	reconcile    *services.ReconcileUseCase
}

func NewReconciliationHandler(
	operations ports.OperationRepository,
	occurrences ports.PlannedOccurrenceRepository,
	plannedItems ports.PlannedItemRepository,
	accounts *AccountHandler,
	reconcile *services.ReconcileUseCase,
) *ReconciliationHandler {
	return &ReconciliationHandler{operations: operations, occurrences: occurrences, plannedItems: plannedItems, accounts: accounts, reconcile: reconcile}
}

type operationSummaryResponse struct {
	ID     string `json:"id"`
	Date   string `json:"date"`
	Label  string `json:"label"`
	Amount string `json:"amount"`
}

func toOperationSummaryResponse(o entities.Operation) operationSummaryResponse {
	return operationSummaryResponse{ID: o.ID.String(), Date: o.Date.String(), Label: o.Label, Amount: o.Amount.Decimal.StringFixed(2)}
}

func (h *ReconciliationHandler) ListUnreconciledOperations(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	ops, err := h.operations.ListUnreconciled(r.Context(), account.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]operationSummaryResponse, len(ops))
	for i, o := range ops {
		out[i] = toOperationSummaryResponse(o)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type suggestionResponse struct {
	Occurrence occurrenceResponse       `json:"occurrence"`
	Operation  operationSummaryResponse `json:"operation"`
}

func (h *ReconciliationHandler) Suggestions(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	suggestions, err := h.reconcile.SuggestMatches(r.Context(), account.ID, 0)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]suggestionResponse, len(suggestions))
	for i, s := range suggestions {
		out[i] = suggestionResponse{Occurrence: toOccurrenceResponse(s.Occurrence, nil), Operation: toOperationSummaryResponse(s.Operation)}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// getOwnedOccurrence returns the occurrence, after checking the current
// user can access the account behind it (via its PlannedItem) — their own,
// or one shared with them, since reconciling is part of a collaborator's
// granted scope.
func (h *ReconciliationHandler) getOwnedOccurrence(r *http.Request, occurrenceID string) (*entities.PlannedOccurrence, error) {
	occID, err := parseUUID(occurrenceID)
	if err != nil {
		return nil, httpx.NotFound("planned occurrence not found")
	}
	occurrence, err := h.occurrences.GetByID(r.Context(), occID)
	if err != nil {
		return nil, err
	}
	if occurrence == nil {
		return nil, httpx.NotFound("planned occurrence not found")
	}
	item, err := h.plannedItems.GetByID(r.Context(), occurrence.PlannedItemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, httpx.NotFound("planned occurrence not found")
	}
	account, err := h.accounts.accounts.GetByID(r.Context(), item.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, httpx.NotFound("planned occurrence not found")
	}
	ok, err := h.accounts.canAccessOwner(r.Context(), CurrentUser(r).ID, account.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.NotFound("planned occurrence not found")
	}
	return occurrence, nil
}

// requireOperationOnOccurrenceAccount answers 404 — never revealing whether
// the operation exists elsewhere — unless the operation belongs to the same
// account as the occurrence's planned item.
func (h *ReconciliationHandler) requireOperationOnOccurrenceAccount(r *http.Request, opID uuid.UUID, occurrence *entities.PlannedOccurrence) error {
	operation, err := h.operations.GetByID(r.Context(), opID)
	if err != nil {
		return err
	}
	item, err := h.plannedItems.GetByID(r.Context(), occurrence.PlannedItemID)
	if err != nil {
		return err
	}
	if operation == nil || item == nil || operation.AccountID != item.AccountID {
		return httpx.NotFound("operation not found")
	}
	return nil
}

type reconcileRequest struct {
	OccurrenceID string `json:"occurrence_id"`
	OperationID  string `json:"operation_id"`
}

func (h *ReconciliationHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	var body reconcileRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	occurrence, err := h.getOwnedOccurrence(r, body.OccurrenceID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	opID, err := parseUUID(body.OperationID)
	if err != nil {
		httpx.WriteError(w, httpx.NotFound("operation not found"))
		return
	}
	// The operation id comes from the request body, so it is checked like
	// any other id: it must exist and sit on the very account the occurrence
	// belongs to (which getOwnedOccurrence already proved the caller can
	// reach). Without this, Reconcile wrote the item's category onto — and
	// echoed the amount of — any operation whose id the caller knew, whoever
	// owned it.
	if err := h.requireOperationOnOccurrenceAccount(r, opID, occurrence); err != nil {
		httpx.WriteError(w, err)
		return
	}
	occID, _ := parseUUID(body.OccurrenceID)
	saved, err := h.reconcile.Reconcile(r.Context(), occID, opID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOccurrenceResponse(*saved, nil))
}

func (h *ReconciliationHandler) Unreconcile(w http.ResponseWriter, r *http.Request) {
	occurrenceIDStr := r.PathValue("occurrenceID")
	if _, err := h.getOwnedOccurrence(r, occurrenceIDStr); err != nil {
		httpx.WriteError(w, err)
		return
	}
	occID, _ := parseUUID(occurrenceIDStr)
	saved, err := h.reconcile.Unreconcile(r.Context(), occID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toOccurrenceResponse(*saved, nil))
}
