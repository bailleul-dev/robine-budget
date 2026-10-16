package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type CommentRepository struct{ pool *pgxpool.Pool }

func NewCommentRepository(pool *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{pool: pool}
}

const commentColumns = "id, operation_id, user_id, body, created_at"

func (r *CommentRepository) ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.Comment, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+commentColumns+" FROM comments WHERE operation_id = $1 ORDER BY created_at", operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Comment
	for rows.Next() {
		var c entities.Comment
		if err := rows.Scan(&c.ID, &c.OperationID, &c.UserID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CommentRepository) ListForAuthor(ctx context.Context, userID uuid.UUID) ([]entities.Comment, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+commentColumns+" FROM comments WHERE user_id = $1 ORDER BY created_at", userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Comment
	for rows.Next() {
		var c entities.Comment
		if err := rows.Scan(&c.ID, &c.OperationID, &c.UserID, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CommentRepository) Add(ctx context.Context, c *entities.Comment) (*entities.Comment, error) {
	var out entities.Comment
	err := r.pool.QueryRow(ctx,
		`INSERT INTO comments (id, operation_id, user_id, body) VALUES ($1, $2, $3, $4)
		 RETURNING `+commentColumns,
		c.ID, c.OperationID, c.UserID, c.Body,
	).Scan(&out.ID, &out.OperationID, &out.UserID, &out.Body, &out.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
