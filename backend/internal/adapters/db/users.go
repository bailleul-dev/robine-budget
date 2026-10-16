package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type UserRepository struct{ pool *pgxpool.Pool }

func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

func scanUser(row pgx.Row) (*entities.User, error) {
	var u entities.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.RobineSub, &u.GoogleSub, &u.AppleSub, &u.DisplayName, &u.CreatedAt, &u.TokenVersion, &u.EmailVerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

const userColumns = "id, email, password_hash, robine_sub, google_sub, apple_sub, display_name, created_at, token_version, email_verified_at"

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.User, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = $1", id)
	return scanUser(row)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*entities.User, error) {
	// Case-insensitive: people type addresses in whatever case they like. If
	// legacy rows differ only by case, the exact match wins.
	row := r.pool.QueryRow(ctx,
		"SELECT "+userColumns+" FROM users WHERE lower(email) = lower($1) ORDER BY (email = $1) DESC LIMIT 1", email)
	return scanUser(row)
}

func (r *UserRepository) GetByRobineSub(ctx context.Context, sub string) (*entities.User, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE robine_sub = $1", sub)
	return scanUser(row)
}

func (r *UserRepository) GetByGoogleSub(ctx context.Context, sub string) (*entities.User, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE google_sub = $1", sub)
	return scanUser(row)
}

func (r *UserRepository) GetByAppleSub(ctx context.Context, sub string) (*entities.User, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE apple_sub = $1", sub)
	return scanUser(row)
}

func (r *UserRepository) Add(ctx context.Context, u *entities.User) (*entities.User, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO users (id, email, password_hash, robine_sub, google_sub, apple_sub, display_name, email_verified_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+userColumns,
		u.ID, u.Email, u.PasswordHash, u.RobineSub, u.GoogleSub, u.AppleSub, u.DisplayName, u.EmailVerifiedAt,
	)
	return scanUser(row)
}

func (r *UserRepository) Update(ctx context.Context, u *entities.User) (*entities.User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET email = $2, password_hash = $3, robine_sub = $4, google_sub = $5, apple_sub = $6, display_name = $7,
		        email_verified_at = $8
		 WHERE id = $1
		 RETURNING `+userColumns,
		u.ID, u.Email, u.PasswordHash, u.RobineSub, u.GoogleSub, u.AppleSub, u.DisplayName, u.EmailVerifiedAt,
	)
	return scanUser(row)
}

// BumpTokenVersion is deliberately not part of Update: several flows load a
// user, change one field and write the whole row back, and a stale copy must
// never be able to "un-revoke" sessions. An atomic increment can't be.
func (r *UserRepository) BumpTokenVersion(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET token_version = token_version + 1 WHERE id = $1`, id)
	return err
}

// Delete permanently removes a user and everything they own (App Store
// in-app account deletion). Most of it is ON DELETE CASCADE from users (see
// migration 000017); the one thing a cascade can't express is planned
// items on *other* users' accounts that point at one of this user's
// custom categories (possible via a shared account): a category-mode
// budget can't exist without its category, so it's deleted, while a
// label-mode item just loses its optional category. Both happen here, in
// the same transaction as the delete itself.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(ctx,
		`DELETE FROM planned_items
		 WHERE match_mode = 'category'
		   AND category_id IN (SELECT id FROM categories WHERE user_id = $1)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE planned_items SET category_id = NULL
		 WHERE category_id IN (SELECT id FROM categories WHERE user_id = $1)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
