package web

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/httpx"
)

type BankProfileHandler struct {
	profiles ports.BankProfileRepository
}

func NewBankProfileHandler(profiles ports.BankProfileRepository) *BankProfileHandler {
	return &BankProfileHandler{profiles: profiles}
}

type columnMappingRequest struct {
	Date      string  `json:"date"`
	Label     string  `json:"label"`
	Amount    *string `json:"amount"`
	Debit     *string `json:"debit"`
	Credit    *string `json:"credit"`
	Reference *string `json:"reference"`
}

func (m columnMappingRequest) toEntity() entities.ColumnMapping {
	return entities.ColumnMapping{Date: m.Date, Label: m.Label, Amount: m.Amount, Debit: m.Debit, Credit: m.Credit, Reference: m.Reference}
}

type bankProfileRequest struct {
	Name             string               `json:"name"`
	Delimiter        string               `json:"delimiter"`
	Encoding         string               `json:"encoding"`
	DateFormat       string               `json:"date_format"`
	DecimalSeparator string               `json:"decimal_separator"`
	AmountMode       string               `json:"amount_mode"`
	ColumnMapping    columnMappingRequest `json:"column_mapping"`
}

type bankProfileResponse struct {
	ID               string               `json:"id"`
	Global           bool                 `json:"global"`
	Name             string               `json:"name"`
	Delimiter        string               `json:"delimiter"`
	Encoding         string               `json:"encoding"`
	DateFormat       string               `json:"date_format"`
	DecimalSeparator string               `json:"decimal_separator"`
	AmountMode       string               `json:"amount_mode"`
	ColumnMapping    columnMappingRequest `json:"column_mapping"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

func toBankProfileResponse(p entities.BankProfile) bankProfileResponse {
	return bankProfileResponse{
		ID: p.ID.String(), Global: p.UserID == nil, Name: p.Name, Delimiter: p.Delimiter, Encoding: p.Encoding,
		DateFormat: p.DateFormat, DecimalSeparator: p.DecimalSeparator, AmountMode: string(p.AmountMode),
		ColumnMapping: columnMappingRequest{
			Date: p.ColumnMapping.Date, Label: p.ColumnMapping.Label, Amount: p.ColumnMapping.Amount,
			Debit: p.ColumnMapping.Debit, Credit: p.ColumnMapping.Credit, Reference: p.ColumnMapping.Reference,
		},
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (h *BankProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.profiles.ListForUser(r.Context(), CurrentUser(r).ID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	out := make([]bankProfileResponse, len(profiles))
	for i, p := range profiles {
		out[i] = toBankProfileResponse(p)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *BankProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body bankProfileRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	amountMode, err := validateAmountMode(body)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	ownerID := CurrentUser(r).ID
	profile := &entities.BankProfile{
		ID: uuid.New(), UserID: &ownerID, Name: body.Name, Delimiter: body.Delimiter,
		Encoding: body.Encoding, DateFormat: body.DateFormat, DecimalSeparator: body.DecimalSeparator,
		AmountMode: amountMode, ColumnMapping: body.ColumnMapping.toEntity(),
	}
	created, err := h.profiles.Add(r.Context(), profile)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toBankProfileResponse(*created))
}

func validateAmountMode(body bankProfileRequest) (entities.AmountMode, error) {
	mode, err := entities.ParseAmountMode(body.AmountMode)
	if err != nil {
		return "", httpx.BadRequest(`amount_mode must be "single", "debit_credit" or "debit_credit_signed"`)
	}
	switch mode {
	case entities.AmountModeSingle:
		if body.ColumnMapping.Amount == nil || *body.ColumnMapping.Amount == "" {
			return "", httpx.BadRequest("amount column is required for amount_mode=single")
		}
	case entities.AmountModeDebitCredit, entities.AmountModeDebitCreditSigned:
		if body.ColumnMapping.Debit == nil || body.ColumnMapping.Credit == nil {
			return "", httpx.BadRequest("debit and credit columns are required for amount_mode=" + string(mode))
		}
	}
	return mode, nil
}

func (h *BankProfileHandler) OwnedProfile(r *http.Request) (*entities.BankProfile, error) {
	id, err := parseUUIDParam(r, "profileID")
	if err != nil {
		return nil, httpx.NotFound("bank profile not found")
	}
	profile, err := h.profiles.GetByID(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.UserID == nil || *profile.UserID != CurrentUser(r).ID {
		return nil, httpx.NotFound("bank profile not found")
	}
	return profile, nil
}

func (h *BankProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, err := h.OwnedProfile(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var body bankProfileRequest
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	amountMode, err := validateAmountMode(body)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	existing.Name = body.Name
	existing.Delimiter = body.Delimiter
	existing.Encoding = body.Encoding
	existing.DateFormat = body.DateFormat
	existing.DecimalSeparator = body.DecimalSeparator
	existing.AmountMode = amountMode
	existing.ColumnMapping = body.ColumnMapping.toEntity()
	saved, err := h.profiles.Update(r.Context(), existing)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toBankProfileResponse(*saved))
}
