package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type AccountRepository struct{ pool *pgxpool.Pool }

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

const accountColumns = "id, user_id, name, currency, created_at, bank_sync_account_id"

func scanAccount(row pgx.Row) (*entities.Account, error) {
	var a entities.Account
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.Currency, &a.CreatedAt, &a.BankSyncAccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.Account, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id = $1", id)
	return scanAccount(row)
}

func (r *AccountRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.Account, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+accountColumns+" FROM accounts WHERE user_id = $1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *AccountRepository) GetByBankSyncAccountID(ctx context.Context, userID uuid.UUID, bankSyncAccountID string) (*entities.Account, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+accountColumns+" FROM accounts WHERE user_id = $1 AND bank_sync_account_id = $2",
		userID, bankSyncAccountID,
	)
	return scanAccount(row)
}

func (r *AccountRepository) Add(ctx context.Context, a *entities.Account) (*entities.Account, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO accounts (id, user_id, name, currency, bank_sync_account_id) VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+accountColumns,
		a.ID, a.UserID, a.Name, a.Currency, a.BankSyncAccountID,
	)
	return scanAccount(row)
}

func (r *AccountRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM accounts WHERE id = $1", id)
	return err
}
