package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type CategoryRepository struct{ pool *pgxpool.Pool }

func NewCategoryRepository(pool *pgxpool.Pool) *CategoryRepository {
	return &CategoryRepository{pool: pool}
}

const categoryColumns = "id, user_id, parent_id, name, created_at"

func scanCategory(row pgx.Row) (*entities.Category, error) {
	var c entities.Category
	err := row.Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CategoryRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.Category, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+categoryColumns+" FROM categories WHERE id = $1", id)
	return scanCategory(row)
}

// ListVisibleTo returns standard categories (user_id IS NULL) plus the
// user's own — mirrors the Python repository's OR-based visibility filter.
func (r *CategoryRepository) ListVisibleTo(ctx context.Context, userID uuid.UUID) ([]entities.Category, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE user_id IS NULL OR user_id = $1", userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Category
	for rows.Next() {
		var c entities.Category
		if err := rows.Scan(&c.ID, &c.UserID, &c.ParentID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CategoryRepository) Add(ctx context.Context, c *entities.Category) (*entities.Category, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO categories (id, user_id, parent_id, name) VALUES ($1, $2, $3, $4)
		 RETURNING `+categoryColumns,
		c.ID, c.UserID, c.ParentID, c.Name,
	)
	return scanCategory(row)
}

func (r *CategoryRepository) Update(ctx context.Context, c *entities.Category) (*entities.Category, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE categories SET parent_id = $2, name = $3 WHERE id = $1 RETURNING `+categoryColumns,
		c.ID, c.ParentID, c.Name,
	)
	return scanCategory(row)
}

func (r *CategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM categories WHERE id = $1", id)
	return err
}
