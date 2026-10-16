package db

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type ImportBatchRepository struct{ pool *pgxpool.Pool }

func NewImportBatchRepository(pool *pgxpool.Pool) *ImportBatchRepository {
	return &ImportBatchRepository{pool: pool}
}

const importBatchColumns = "id, account_id, bank_profile_id, filename, imported_at, row_count, imported_count, duplicate_count, errors"

func (r *ImportBatchRepository) Add(ctx context.Context, b *entities.ImportBatch) (*entities.ImportBatch, error) {
	errorsJSON, err := json.Marshal(b.Errors)
	if err != nil {
		return nil, err
	}
	var out entities.ImportBatch
	var errorsRaw []byte
	err = r.pool.QueryRow(ctx,
		`INSERT INTO import_batches (id, account_id, bank_profile_id, filename, row_count, imported_count, duplicate_count, errors)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+importBatchColumns,
		b.ID, b.AccountID, b.BankProfileID, b.Filename, b.RowCount, b.ImportedCount, b.DuplicateCount, errorsJSON,
	).Scan(&out.ID, &out.AccountID, &out.BankProfileID, &out.Filename, &out.ImportedAt, &out.RowCount, &out.ImportedCount, &out.DuplicateCount, &errorsRaw)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(errorsRaw, &out.Errors); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *ImportBatchRepository) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.ImportBatch, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+importBatchColumns+" FROM import_batches WHERE account_id = $1", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.ImportBatch
	for rows.Next() {
		var b entities.ImportBatch
		var errorsRaw []byte
		if err := rows.Scan(&b.ID, &b.AccountID, &b.BankProfileID, &b.Filename, &b.ImportedAt, &b.RowCount, &b.ImportedCount, &b.DuplicateCount, &errorsRaw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(errorsRaw, &b.Errors); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
