package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/pkg/civil"
)

type OperationRepository struct{ pool *pgxpool.Pool }

func NewOperationRepository(pool *pgxpool.Pool) *OperationRepository {
	return &OperationRepository{pool: pool}
}

const operationColumns = "id, account_id, import_batch_id, date, label, amount, balance_after, raw_reference, dedup_hash, created_at, category_id"

func scanOperation(row pgx.Row) (*entities.Operation, error) {
	var o entities.Operation
	err := row.Scan(&o.ID, &o.AccountID, &o.ImportBatchID, &o.Date, &o.Label, &o.Amount, &o.BalanceAfter, &o.RawReference, &o.DedupHash, &o.CreatedAt, &o.CategoryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func scanOperationRow(rows pgx.Rows) (entities.Operation, error) {
	var o entities.Operation
	err := rows.Scan(&o.ID, &o.AccountID, &o.ImportBatchID, &o.Date, &o.Label, &o.Amount, &o.BalanceAfter, &o.RawReference, &o.DedupHash, &o.CreatedAt, &o.CategoryID)
	return o, err
}

func (r *OperationRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.Operation, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+operationColumns+" FROM operations WHERE id = $1", id)
	return scanOperation(row)
}

func (r *OperationRepository) FindByReference(ctx context.Context, accountID uuid.UUID, rawReference string) (*entities.Operation, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+operationColumns+" FROM operations WHERE account_id = $1 AND raw_reference = $2",
		accountID, rawReference,
	)
	return scanOperation(row)
}

func (r *OperationRepository) FindByDedupHash(ctx context.Context, accountID uuid.UUID, dedupHash string) (*entities.Operation, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+operationColumns+" FROM operations WHERE account_id = $1 AND dedup_hash = $2",
		accountID, dedupHash,
	)
	return scanOperation(row)
}

func (r *OperationRepository) Add(ctx context.Context, op *entities.Operation) (*entities.Operation, error) {
	result, err := r.AddMany(ctx, []entities.Operation{*op})
	if err != nil {
		return nil, err
	}
	return &result[0], nil
}

func (r *OperationRepository) AddMany(ctx context.Context, ops []entities.Operation) ([]entities.Operation, error) {
	out := make([]entities.Operation, 0, len(ops))
	for _, op := range ops {
		row := r.pool.QueryRow(ctx,
			`INSERT INTO operations (id, account_id, import_batch_id, date, label, amount, balance_after, raw_reference, dedup_hash, category_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 RETURNING `+operationColumns,
			op.ID, op.AccountID, op.ImportBatchID, op.Date, op.Label, op.Amount, op.BalanceAfter, op.RawReference, op.DedupHash, op.CategoryID,
		)
		saved, err := scanOperation(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *saved)
	}
	return out, nil
}

// ListForAccount: newest first (matches the Python repository — callers that
// need ascending order, like PrevisionsUseCase, re-sort themselves).
func (r *OperationRepository) ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.Operation, error) {
	var sb strings.Builder
	sb.WriteString("SELECT " + operationColumns + " FROM operations WHERE account_id = $1")
	args := []any{accountID}
	if dateFrom != nil {
		args = append(args, *dateFrom)
		sb.WriteString(fmt.Sprintf(" AND date >= $%d", len(args)))
	}
	if dateTo != nil {
		args = append(args, *dateTo)
		sb.WriteString(fmt.Sprintf(" AND date <= $%d", len(args)))
	}
	sb.WriteString(" ORDER BY date DESC, created_at DESC")

	rows, err := r.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Operation
	for rows.Next() {
		o, err := scanOperationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ListPageForAccount keyset-paginates: ORDER BY date DESC, created_at DESC,
// id DESC (id as final tiebreaker since two operations from the same import
// batch can share a created_at down to the microsecond), resuming strictly
// after the cursor's row via a row-value comparison rather than an OFFSET —
// stays cheap and stable deep into an account's history.
func (r *OperationRepository) ListPageForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date, uncategorizedOnly bool, after *entities.OperationPageCursor, limit int) ([]entities.Operation, error) {
	var sb strings.Builder
	sb.WriteString("SELECT " + operationColumns + " FROM operations WHERE account_id = $1")
	args := []any{accountID}
	if dateFrom != nil {
		args = append(args, *dateFrom)
		sb.WriteString(fmt.Sprintf(" AND date >= $%d", len(args)))
	}
	if dateTo != nil {
		args = append(args, *dateTo)
		sb.WriteString(fmt.Sprintf(" AND date <= $%d", len(args)))
	}
	if uncategorizedOnly {
		sb.WriteString(" AND category_id IS NULL AND NOT EXISTS (SELECT 1 FROM operation_category_splits s WHERE s.operation_id = operations.id)")
	}
	if after != nil {
		args = append(args, after.Date, after.CreatedAt, after.ID)
		sb.WriteString(fmt.Sprintf(" AND (date, created_at, id) < ($%d, $%d, $%d)", len(args)-2, len(args)-1, len(args)))
	}
	args = append(args, limit)
	sb.WriteString(fmt.Sprintf(" ORDER BY date DESC, created_at DESC, id DESC LIMIT $%d", len(args)))

	rows, err := r.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Operation
	for rows.Next() {
		o, err := scanOperationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *OperationRepository) ListUnreconciled(ctx context.Context, accountID uuid.UUID) ([]entities.Operation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+operationColumns+` FROM operations
		 WHERE account_id = $1
		   AND id NOT IN (
		     SELECT reconciled_operation_id FROM planned_occurrences
		     WHERE reconciled_operation_id IS NOT NULL
		   )`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.Operation
	for rows.Next() {
		o, err := scanOperationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *OperationRepository) Update(ctx context.Context, op *entities.Operation) (*entities.Operation, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE operations SET label = $2, amount = $3, balance_after = $4, category_id = $5
		 WHERE id = $1
		 RETURNING `+operationColumns,
		op.ID, op.Label, op.Amount, op.BalanceAfter, op.CategoryID,
	)
	return scanOperation(row)
}

// CategoryCountsForLabels: lower(trim(...)) matches normalizeLabel's own
// Go-side normalization exactly (see services.normalizeLabel) - both must
// stay in lockstep, since the caller matches these rows back up by
// re-computing the same normalization in Go. Joins to accounts for
// user_id since operations don't carry it directly - a normalized label is
// looked up across every account in the system, not just the caller's own.
func (r *OperationRepository) CategoryCountsForLabels(ctx context.Context, normalizedLabels []string) ([]ports.LabelCategoryCount, error) {
	if len(normalizedLabels) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT lower(trim(o.label)) AS norm_label, a.user_id, o.category_id, count(*)
		 FROM operations o
		 JOIN accounts a ON a.id = o.account_id
		 WHERE lower(trim(o.label)) = ANY($1) AND o.category_id IS NOT NULL
		 GROUP BY norm_label, a.user_id, o.category_id`,
		normalizedLabels,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ports.LabelCategoryCount
	for rows.Next() {
		var c ports.LabelCategoryCount
		if err := rows.Scan(&c.NormalizedLabel, &c.OwnerUserID, &c.CategoryID, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
