package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
	"bailleul.dev/budget-server/pkg/civil"
	"bailleul.dev/budget-server/pkg/httpx"
	"bailleul.dev/budget-server/pkg/money"
)

type BalanceCheckpointHandler struct {
	checkpoints    ports.BalanceCheckpointRepository
	accounts       *AccountHandler
	computeInitial *services.ComputeInitialBalanceUseCase
}

func NewBalanceCheckpointHandler(
	checkpoints ports.BalanceCheckpointRepository,
	accounts *AccountHandler,
	computeInitial *services.ComputeInitialBalanceUseCase,
) *BalanceCheckpointHandler {
	return &BalanceCheckpointHandler{checkpoints: checkpoints, accounts: accounts, computeInitial: computeInitial}
}

type balanceCheckpointResponse struct {
	ID        string    `json:"id"`
	AsOfDate  string    `json:"as_of_date"`
	Balance   string    `json:"balance"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

func toCheckpointResponse(c entities.BalanceCheckpoint) balanceCheckpointResponse {
	return balanceCheckpointResponse{
		ID: c.ID.String(), AsOfDate: c.AsOfDate.String(), Balance: c.Balance.Decimal.StringFixed(2),
		Note: c.Note, CreatedAt: c.CreatedAt,
	}
}

func (h *BalanceCheckpointHandler) List(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.AccessibleAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	checkpoints, err := h.checkpoints.ListForAccount(r.Context(), account.ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]balanceCheckpointResponse, len(checkpoints))
	for i, c := range checkpoints {
		out[i] = toCheckpointResponse(c)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type createCheckpointRequest struct {
	AsOfDate string  `json:"as_of_date"`
	Balance  string  `json:"balance"`
	Note     *string `json:"note"`
}

func (h *BalanceCheckpointHandler) Create(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.RequireOwnAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body createCheckpointRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	asOf, err := civil.Parse(body.AsOfDate)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid as_of_date"))
		return
	}
	balance, err := money.New(body.Balance)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid balance"))
		return
	}
	checkpoint := &entities.BalanceCheckpoint{
		ID: uuid.New(), AccountID: account.ID, AsOfDate: asOf, Balance: balance, Note: body.Note,
	}
	created, err := h.checkpoints.Add(r.Context(), checkpoint)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCheckpointResponse(*created))
}

// ComputeInitial bootstraps the account's first-ever checkpoint from a
// balance the user knows to be true right now — see
// ComputeInitialBalanceUseCase. Only valid while the account has no
// checkpoint yet; afterward, corrections go through the plain Create above.
func (h *BalanceCheckpointHandler) ComputeInitial(w http.ResponseWriter, r *http.Request) {
	account, err := h.accounts.RequireOwnAccount(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body createCheckpointRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	asOf, err := civil.Parse(body.AsOfDate)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid as_of_date"))
		return
	}
	balance, err := money.New(body.Balance)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("invalid balance"))
		return
	}
	created, err := h.computeInitial.Execute(r.Context(), account.ID, asOf, balance, body.Note)
	if err != nil {
		if errors.Is(err, services.ErrInitialBalanceAlreadySet) {
			httpx.WriteError(w, httpx.BadRequest(err.Error()))
			return
		}
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCheckpointResponse(*created))
}

func (h *BalanceCheckpointHandler) OwnedCheckpoint(r *http.Request) (*entities.BalanceCheckpoint, error) {
	id, err := parseUUIDParam(r, "checkpointID")
	if err != nil {
		return nil, httpx.NotFound("balance checkpoint not found")
	}
	checkpoint, err := h.checkpoints.GetByID(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, httpx.NotFound("balance checkpoint not found")
	}
	account, err := h.accounts.accounts.GetByID(r.Context(), checkpoint.AccountID)
	if err != nil {
		return nil, err
	}
	if account == nil || account.UserID != CurrentUser(r).ID {
		return nil, httpx.NotFound("balance checkpoint not found")
	}
	return checkpoint, nil
}

func (h *BalanceCheckpointHandler) Delete(w http.ResponseWriter, r *http.Request) {
	checkpoint, err := h.OwnedCheckpoint(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := h.checkpoints.Delete(r.Context(), checkpoint.ID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
