package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/pkg/civil"
)

type BalanceCheckpointRepository struct{ pool *pgxpool.Pool }

func NewBalanceCheckpointRepository(pool *pgxpool.Pool) *BalanceCheckpointRepository {
	return &BalanceCheckpointRepository{pool: pool}
}

const checkpointColumns = "id, account_id, as_of_date, balance, note, created_at"

func scanCheckpoint(row pgx.Row) (*entities.BalanceCheckpoint, error) {
	var c entities.BalanceCheckpoint
	err := row.Scan(&c.ID, &c.AccountID, &c.AsOfDate, &c.Balance, &c.Note, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *BalanceCheckpointRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.BalanceCheckpoint, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+checkpointColumns+" FROM balance_checkpoints WHERE id = $1", id)
	return scanCheckpoint(row)
}

func (r *BalanceCheckpointRepository) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.BalanceCheckpoint, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+checkpointColumns+" FROM balance_checkpoints WHERE account_id = $1 ORDER BY as_of_date DESC",
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.BalanceCheckpoint
	for rows.Next() {
		var c entities.BalanceCheckpoint
		if err := rows.Scan(&c.ID, &c.AccountID, &c.AsOfDate, &c.Balance, &c.Note, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *BalanceCheckpointRepository) GetLatestOnOrBefore(ctx context.Context, accountID uuid.UUID, asOf civil.Date) (*entities.BalanceCheckpoint, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+checkpointColumns+` FROM balance_checkpoints
		 WHERE account_id = $1 AND as_of_date <= $2
		 ORDER BY as_of_date DESC LIMIT 1`,
		accountID, asOf,
	)
	return scanCheckpoint(row)
}

func (r *BalanceCheckpointRepository) Add(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO balance_checkpoints (id, account_id, as_of_date, balance, note)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+checkpointColumns,
		c.ID, c.AccountID, c.AsOfDate, c.Balance, c.Note,
	)
	return scanCheckpoint(row)
}

func (r *BalanceCheckpointRepository) Upsert(ctx context.Context, c *entities.BalanceCheckpoint) (*entities.BalanceCheckpoint, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO balance_checkpoints (id, account_id, as_of_date, balance, note)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (account_id, as_of_date) DO UPDATE SET balance = EXCLUDED.balance, note = EXCLUDED.note
		 RETURNING `+checkpointColumns,
		c.ID, c.AccountID, c.AsOfDate, c.Balance, c.Note,
	)
	return scanCheckpoint(row)
}

func (r *BalanceCheckpointRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM balance_checkpoints WHERE id = $1", id)
	return err
}
