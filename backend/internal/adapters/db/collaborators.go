package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type CollaboratorRepository struct{ pool *pgxpool.Pool }

func NewCollaboratorRepository(pool *pgxpool.Pool) *CollaboratorRepository {
	return &CollaboratorRepository{pool: pool}
}

const collaboratorColumns = "id, owner_user_id, invited_email, collaborator_user_id, role, token, created_at, accepted_at, revoked_at"

func scanCollaborator(row pgx.Row) (*entities.Collaborator, error) {
	var c entities.Collaborator
	err := row.Scan(&c.ID, &c.OwnerUserID, &c.InvitedEmail, &c.CollaboratorUserID, &c.Role, &c.Token, &c.CreatedAt, &c.AcceptedAt, &c.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func scanCollaboratorRow(rows pgx.Rows) (entities.Collaborator, error) {
	var c entities.Collaborator
	err := rows.Scan(&c.ID, &c.OwnerUserID, &c.InvitedEmail, &c.CollaboratorUserID, &c.Role, &c.Token, &c.CreatedAt, &c.AcceptedAt, &c.RevokedAt)
	return c, err
}

func (r *CollaboratorRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.Collaborator, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+collaboratorColumns+" FROM collaborators WHERE id = $1", id)
	return scanCollaborator(row)
}

func (r *CollaboratorRepository) GetByToken(ctx context.Context, token string) (*entities.Collaborator, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+collaboratorColumns+" FROM collaborators WHERE token = $1", token)
	return scanCollaborator(row)
}

func (r *CollaboratorRepository) ListForOwner(ctx context.Context, ownerUserID uuid.UUID) ([]entities.Collaborator, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+collaboratorColumns+" FROM collaborators WHERE owner_user_id = $1 ORDER BY created_at DESC", ownerUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Collaborator
	for rows.Next() {
		c, err := scanCollaboratorRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CollaboratorRepository) ListActiveForCollaborator(ctx context.Context, collaboratorUserID uuid.UUID) ([]entities.Collaborator, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+collaboratorColumns+` FROM collaborators
		 WHERE collaborator_user_id = $1 AND accepted_at IS NOT NULL AND revoked_at IS NULL`,
		collaboratorUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Collaborator
	for rows.Next() {
		c, err := scanCollaboratorRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CollaboratorRepository) Add(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO collaborators (id, owner_user_id, invited_email, collaborator_user_id, role, token, accepted_at, revoked_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+collaboratorColumns,
		c.ID, c.OwnerUserID, c.InvitedEmail, c.CollaboratorUserID, c.Role, c.Token, c.AcceptedAt, c.RevokedAt,
	)
	return scanCollaborator(row)
}

func (r *CollaboratorRepository) Update(ctx context.Context, c *entities.Collaborator) (*entities.Collaborator, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE collaborators SET collaborator_user_id = $2, accepted_at = $3, revoked_at = $4
		 WHERE id = $1
		 RETURNING `+collaboratorColumns,
		c.ID, c.CollaboratorUserID, c.AcceptedAt, c.RevokedAt,
	)
	return scanCollaborator(row)
}
