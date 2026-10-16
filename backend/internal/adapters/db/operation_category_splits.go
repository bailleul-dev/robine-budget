package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type OperationCategorySplitRepository struct{ pool *pgxpool.Pool }

func NewOperationCategorySplitRepository(pool *pgxpool.Pool) *OperationCategorySplitRepository {
	return &OperationCategorySplitRepository{pool: pool}
}

const operationCategorySplitColumns = "id, operation_id, category_id, amount"

func scanOperationCategorySplitRow(rows pgx.Rows) (entities.OperationCategorySplit, error) {
	var s entities.OperationCategorySplit
	err := rows.Scan(&s.ID, &s.OperationID, &s.CategoryID, &s.Amount)
	return s, err
}

func (r *OperationCategorySplitRepository) ListForOperation(ctx context.Context, operationID uuid.UUID) ([]entities.OperationCategorySplit, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT "+operationCategorySplitColumns+" FROM operation_category_splits WHERE operation_id = $1 ORDER BY amount DESC",
		operationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.OperationCategorySplit
	for rows.Next() {
		s, err := scanOperationCategorySplitRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *OperationCategorySplitRepository) ListForOperationIDs(ctx context.Context, operationIDs []uuid.UUID) ([]entities.OperationCategorySplit, error) {
	if len(operationIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(operationIDs))
	args := make([]any, len(operationIDs))
	for i, id := range operationIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := "SELECT " + operationCategorySplitColumns + " FROM operation_category_splits WHERE operation_id IN (" +
		strings.Join(placeholders, ", ") + ") ORDER BY amount DESC"
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.OperationCategorySplit
	for rows.Next() {
		s, err := scanOperationCategorySplitRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *OperationCategorySplitRepository) ReplaceForOperation(ctx context.Context, operationID uuid.UUID, splits []entities.OperationCategorySplit) ([]entities.OperationCategorySplit, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "DELETE FROM operation_category_splits WHERE operation_id = $1", operationID); err != nil {
		return nil, err
	}

	out := make([]entities.OperationCategorySplit, 0, len(splits))
	for _, s := range splits {
		row := tx.QueryRow(ctx,
			`INSERT INTO operation_category_splits (id, operation_id, category_id, amount)
			 VALUES ($1, $2, $3, $4)
			 RETURNING `+operationCategorySplitColumns,
			uuid.New(), operationID, s.CategoryID, s.Amount,
		)
		var saved entities.OperationCategorySplit
		if err := row.Scan(&saved.ID, &saved.OperationID, &saved.CategoryID, &saved.Amount); err != nil {
			return nil, err
		}
		out = append(out, saved)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}
