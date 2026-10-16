package db

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type BankProfileRepository struct{ pool *pgxpool.Pool }

func NewBankProfileRepository(pool *pgxpool.Pool) *BankProfileRepository {
	return &BankProfileRepository{pool: pool}
}

// columnMappingJSON mirrors the JSON shape stored in bank_profiles.column_mapping.
type columnMappingJSON struct {
	Date      string  `json:"date"`
	Label     string  `json:"label"`
	Amount    *string `json:"amount"`
	Debit     *string `json:"debit"`
	Credit    *string `json:"credit"`
	Reference *string `json:"reference"`
}

func toColumnMappingJSON(m entities.ColumnMapping) columnMappingJSON {
	return columnMappingJSON{Date: m.Date, Label: m.Label, Amount: m.Amount, Debit: m.Debit, Credit: m.Credit, Reference: m.Reference}
}

func (m columnMappingJSON) toEntity() entities.ColumnMapping {
	return entities.ColumnMapping{Date: m.Date, Label: m.Label, Amount: m.Amount, Debit: m.Debit, Credit: m.Credit, Reference: m.Reference}
}

const bankProfileColumns = "id, user_id, name, delimiter, encoding, date_format, decimal_separator, amount_mode, column_mapping, created_at, updated_at"

func scanBankProfile(row pgx.Row) (*entities.BankProfile, error) {
	var p entities.BankProfile
	var mappingRaw []byte
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Delimiter, &p.Encoding, &p.DateFormat, &p.DecimalSeparator, &p.AmountMode, &mappingRaw, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var mapping columnMappingJSON
	if err := json.Unmarshal(mappingRaw, &mapping); err != nil {
		return nil, err
	}
	p.ColumnMapping = mapping.toEntity()
	return &p, nil
}

func (r *BankProfileRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.BankProfile, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+bankProfileColumns+" FROM bank_profiles WHERE id = $1", id)
	return scanBankProfile(row)
}

// ListForUser returns every global profile (user_id IS NULL, shared with all
// users) plus the custom profiles owned by userID.
func (r *BankProfileRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.BankProfile, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+bankProfileColumns+" FROM bank_profiles WHERE user_id IS NULL OR user_id = $1 ORDER BY user_id NULLS FIRST, name",
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.BankProfile
	for rows.Next() {
		p, err := scanBankProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *BankProfileRepository) Add(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	mappingJSON, err := json.Marshal(toColumnMappingJSON(p.ColumnMapping))
	if err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx,
		`INSERT INTO bank_profiles (id, user_id, name, delimiter, encoding, date_format, decimal_separator, amount_mode, column_mapping)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+bankProfileColumns,
		p.ID, p.UserID, p.Name, p.Delimiter, p.Encoding, p.DateFormat, p.DecimalSeparator, p.AmountMode, mappingJSON,
	)
	return scanBankProfile(row)
}

func (r *BankProfileRepository) Update(ctx context.Context, p *entities.BankProfile) (*entities.BankProfile, error) {
	mappingJSON, err := json.Marshal(toColumnMappingJSON(p.ColumnMapping))
	if err != nil {
		return nil, err
	}
	row := r.pool.QueryRow(ctx,
		`UPDATE bank_profiles SET name = $2, delimiter = $3, encoding = $4, date_format = $5,
		   decimal_separator = $6, amount_mode = $7, column_mapping = $8, updated_at = now()
		 WHERE id = $1
		 RETURNING `+bankProfileColumns,
		p.ID, p.Name, p.Delimiter, p.Encoding, p.DateFormat, p.DecimalSeparator, p.AmountMode, mappingJSON,
	)
	return scanBankProfile(row)
}
