package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type BankConnectionRepository struct{ pool *pgxpool.Pool }

func NewBankConnectionRepository(pool *pgxpool.Pool) *BankConnectionRepository {
	return &BankConnectionRepository{pool: pool}
}

const bankConnectionColumns = "id, user_id, bank, login_encrypted, password_encrypted, session_encrypted, created_at, updated_at"

func scanBankConnection(row pgx.Row) (*entities.BankConnection, error) {
	var c entities.BankConnection
	err := row.Scan(&c.ID, &c.UserID, &c.Bank, &c.LoginEncrypted, &c.PasswordEncrypted, &c.SessionEncrypted, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *BankConnectionRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*entities.BankConnection, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+bankConnectionColumns+" FROM bank_connections WHERE user_id = $1", userID)
	return scanBankConnection(row)
}

func (r *BankConnectionRepository) Add(ctx context.Context, c *entities.BankConnection) (*entities.BankConnection, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO bank_connections (id, user_id, bank, login_encrypted, password_encrypted, session_encrypted)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+bankConnectionColumns,
		c.ID, c.UserID, c.Bank, c.LoginEncrypted, c.PasswordEncrypted, c.SessionEncrypted,
	)
	return scanBankConnection(row)
}

func (r *BankConnectionRepository) UpdateSession(ctx context.Context, id uuid.UUID, sessionEncrypted []byte) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE bank_connections SET session_encrypted = $2, updated_at = now() WHERE id = $1",
		id, sessionEncrypted,
	)
	return err
}

func (r *BankConnectionRepository) ListUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, "SELECT user_id FROM bank_connections")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
