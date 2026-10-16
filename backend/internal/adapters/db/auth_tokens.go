package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type AuthTokenRepository struct{ pool *pgxpool.Pool }

func NewAuthTokenRepository(pool *pgxpool.Pool) *AuthTokenRepository {
	return &AuthTokenRepository{pool: pool}
}

func (r *AuthTokenRepository) Create(ctx context.Context, t *entities.AuthToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO auth_tokens (id, user_id, kind, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.Kind, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	return err
}

// Consume flips used_at in the same statement that finds the token, so two
// concurrent requests presenting the same link can't both succeed.
func (r *AuthTokenRepository) Consume(ctx context.Context, kind entities.AuthTokenKind, tokenHash []byte, now time.Time) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx,
		`UPDATE auth_tokens SET used_at = $3
		 WHERE token_hash = $2 AND kind = $1 AND used_at IS NULL AND expires_at > $3
		 RETURNING user_id`, kind, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}

func (r *AuthTokenRepository) InvalidateForUser(ctx context.Context, userID uuid.UUID, kind entities.AuthTokenKind, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE auth_tokens SET used_at = $3 WHERE user_id = $1 AND kind = $2 AND used_at IS NULL`,
		userID, kind, now)
	return err
}
