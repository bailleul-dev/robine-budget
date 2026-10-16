package web

import (
	"net/http"

	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/httpx"
)

type PrevisionsHandler struct {
	previsions *services.PrevisionsUseCase
	accounts   *AccountHandler
}

func NewPrevisionsHandler(previsions *services.PrevisionsUseCase, accounts *AccountHandler) *PrevisionsHandler {
	return &PrevisionsHandler{previsions: previsions, accounts: accounts}
}

type previsionEntryResponse struct {
	Date                string  `json:"date"`
	Amount              string  `json:"amount"`
	Kind                string  `json:"kind"`
	OperationID         *string `json:"operation_id"`
	PlannedOccurrenceID *string `json:"planned_occurrence_id"`
	PlannedItemID       *string `json:"planned_item_id"`
	BalanceCheckpointID *string `json:"balance_checkpoint_id"`
	RunningBalance      string  `json:"running_balance"`
}

type previsionsResponse struct {
	OpeningBalance string                   `json:"opening_balance"`
	ClosingBalance string                   `json:"closing_balance"`
	Entries        []previsionEntryResponse `json:"entries"`
}

func (h *PrevisionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	dateFromRaw := r.URL.Query().Get("date_from")
	dateToRaw := r.URL.Query().Get("date_to")
	if dateFromRaw == "" || dateToRaw == "" {
		httpx.WriteError(w, httpx.BadRequest("date_from and date_to are required"))
		return
	}
	dateFrom, err := civil.Parse(dateFromRaw)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid date_from"))
		return
	}
	dateTo, err := civil.Parse(dateToRaw)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid date_to"))
		return
	}

	result, err := h.previsions.Execute(r.Context(), account.ID, dateFrom, dateTo)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	entries := make([]previsionEntryResponse, len(result.Entries))
	for i, e := range result.Entries {
		var opID, occID, itemID, checkpointID *string
		if e.OperationID != nil {
			s := e.OperationID.String()
			opID = &s
		}
		if e.PlannedOccurrenceID != nil {
			s := e.PlannedOccurrenceID.String()
			occID = &s
		}
		if e.PlannedItemID != nil {
			s := e.PlannedItemID.String()
			itemID = &s
		}
		if e.BalanceCheckpointID != nil {
			s := e.BalanceCheckpointID.String()
			checkpointID = &s
		}
		entries[i] = previsionEntryResponse{
			Date: e.Date.String(), Amount: e.Amount.Decimal.StringFixed(2), Kind: string(e.Kind),
			OperationID: opID, PlannedOccurrenceID: occID, PlannedItemID: itemID, BalanceCheckpointID: checkpointID,
			RunningBalance: e.RunningBalance.Decimal.StringFixed(2),
		}
	}

	httpx.WriteJSON(w, http.StatusOK, previsionsResponse{
		OpeningBalance: result.OpeningBalance.Decimal.StringFixed(2),
		ClosingBalance: result.ClosingBalance.Decimal.StringFixed(2),
		Entries:        entries,
	})
}
