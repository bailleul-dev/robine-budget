package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type OAuthClientRepository struct{ pool *pgxpool.Pool }

func NewOAuthClientRepository(pool *pgxpool.Pool) *OAuthClientRepository {
	return &OAuthClientRepository{pool: pool}
}

const oauthClientColumns = "id, secret_hash, name, redirect_uris, created_at"

func scanOAuthClient(row pgx.Row) (*entities.OAuthClient, error) {
	var c entities.OAuthClient
	if err := row.Scan(&c.ID, &c.SecretHash, &c.Name, &c.RedirectURIs, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *OAuthClientRepository) GetByID(ctx context.Context, id string) (*entities.OAuthClient, error) {
	return scanOAuthClient(r.pool.QueryRow(ctx, "SELECT "+oauthClientColumns+" FROM oauth_clients WHERE id = $1", id))
}

func (r *OAuthClientRepository) Add(ctx context.Context, c *entities.OAuthClient) (*entities.OAuthClient, error) {
	return scanOAuthClient(r.pool.QueryRow(ctx,
		`INSERT INTO oauth_clients (id, secret_hash, name, redirect_uris) VALUES ($1, $2, $3, $4)
		 RETURNING `+oauthClientColumns,
		c.ID, c.SecretHash, c.Name, c.RedirectURIs,
	))
}

type OAuthAuthorizationCodeRepository struct{ pool *pgxpool.Pool }

func NewOAuthAuthorizationCodeRepository(pool *pgxpool.Pool) *OAuthAuthorizationCodeRepository {
	return &OAuthAuthorizationCodeRepository{pool: pool}
}

// Add also sweeps out codes that expired unused (the user never finished,
// or the client never redeemed them) — there's no other cleanup job.
func (r *OAuthAuthorizationCodeRepository) Add(ctx context.Context, c *entities.OAuthAuthorizationCode) error {
	if _, err := r.pool.Exec(ctx, "DELETE FROM oauth_authorization_codes WHERE expires_at < now()"); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO oauth_authorization_codes (code_hash, client_id, user_id, redirect_uri, code_challenge, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.CodeHash, c.ClientID, c.UserID, c.RedirectURI, c.CodeChallenge, c.ExpiresAt,
	)
	return err
}

func (r *OAuthAuthorizationCodeRepository) Consume(ctx context.Context, codeHash string) (*entities.OAuthAuthorizationCode, error) {
	var c entities.OAuthAuthorizationCode
	err := r.pool.QueryRow(ctx,
		`DELETE FROM oauth_authorization_codes WHERE code_hash = $1
		 RETURNING code_hash, client_id, user_id, redirect_uri, code_challenge, expires_at`,
		codeHash,
	).Scan(&c.CodeHash, &c.ClientID, &c.UserID, &c.RedirectURI, &c.CodeChallenge, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

type OAuthGrantRepository struct{ pool *pgxpool.Pool }

func NewOAuthGrantRepository(pool *pgxpool.Pool) *OAuthGrantRepository {
	return &OAuthGrantRepository{pool: pool}
}

const oauthGrantSelect = `SELECT g.id, g.client_id, c.name, g.user_id, g.access_token_hash, g.access_expires_at,
	g.refresh_token_hash, g.refresh_expires_at, g.created_at, g.refreshed_at
	FROM oauth_grants g JOIN oauth_clients c ON c.id = g.client_id`

func scanOAuthGrant(row pgx.Row) (entities.OAuthGrant, error) {
	var g entities.OAuthGrant
	err := row.Scan(&g.ID, &g.ClientID, &g.ClientName, &g.UserID, &g.AccessTokenHash, &g.AccessExpiresAt,
		&g.RefreshTokenHash, &g.RefreshExpiresAt, &g.CreatedAt, &g.RefreshedAt)
	return g, err
}

func (r *OAuthGrantRepository) getOne(ctx context.Context, where string, arg any) (*entities.OAuthGrant, error) {
	g, err := scanOAuthGrant(r.pool.QueryRow(ctx, oauthGrantSelect+" WHERE "+where, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// Add also sweeps out grants whose refresh token has expired: the client
// can never use them again.
func (r *OAuthGrantRepository) Add(ctx context.Context, g *entities.OAuthGrant) (*entities.OAuthGrant, error) {
	if _, err := r.pool.Exec(ctx, "DELETE FROM oauth_grants WHERE refresh_expires_at < now()"); err != nil {
		return nil, err
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO oauth_grants (id, client_id, user_id, access_token_hash, access_expires_at,
		   refresh_token_hash, refresh_expires_at, created_at, refreshed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		g.ID, g.ClientID, g.UserID, g.AccessTokenHash, g.AccessExpiresAt,
		g.RefreshTokenHash, g.RefreshExpiresAt, g.CreatedAt, g.RefreshedAt,
	)
	if err != nil {
		return nil, err
	}
	return r.getOne(ctx, "g.id = $1", g.ID)
}

func (r *OAuthGrantRepository) GetByAccessTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	return r.getOne(ctx, "g.access_token_hash = $1", hash)
}

func (r *OAuthGrantRepository) GetByRefreshTokenHash(ctx context.Context, hash string) (*entities.OAuthGrant, error) {
	return r.getOne(ctx, "g.refresh_token_hash = $1", hash)
}

func (r *OAuthGrantRepository) Rotate(ctx context.Context, id uuid.UUID, oldRefreshHash string, g *entities.OAuthGrant) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE oauth_grants SET access_token_hash = $3, access_expires_at = $4,
		   refresh_token_hash = $5, refresh_expires_at = $6, refreshed_at = $7
		 WHERE id = $1 AND refresh_token_hash = $2`,
		id, oldRefreshHash, g.AccessTokenHash, g.AccessExpiresAt, g.RefreshTokenHash, g.RefreshExpiresAt, g.RefreshedAt,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (r *OAuthGrantRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.OAuthGrant, error) {
	rows, err := r.pool.Query(ctx, oauthGrantSelect+" WHERE g.user_id = $1 ORDER BY g.created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.OAuthGrant
	for rows.Next() {
		g, err := scanOAuthGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *OAuthGrantRepository) Delete(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, "DELETE FROM oauth_grants WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
