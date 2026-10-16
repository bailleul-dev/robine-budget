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
	"bailleul.dev/budget-server/pkg/civil"
)

type PlannedOccurrenceRepository struct{ pool *pgxpool.Pool }

func NewPlannedOccurrenceRepository(pool *pgxpool.Pool) *PlannedOccurrenceRepository {
	return &PlannedOccurrenceRepository{pool: pool}
}

const occurrenceColumns = "id, planned_item_id, due_date, expected_amount, status, reconciled_operation_id, reconciled_amount"

func scanOccurrence(row pgx.Row) (*entities.PlannedOccurrence, error) {
	var o entities.PlannedOccurrence
	err := row.Scan(&o.ID, &o.PlannedItemID, &o.DueDate, &o.ExpectedAmount, &o.Status, &o.ReconciledOperationID, &o.ReconciledAmount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func scanOccurrenceRow(rows pgx.Rows) (entities.PlannedOccurrence, error) {
	var o entities.PlannedOccurrence
	err := rows.Scan(&o.ID, &o.PlannedItemID, &o.DueDate, &o.ExpectedAmount, &o.Status, &o.ReconciledOperationID, &o.ReconciledAmount)
	return o, err
}

func (r *PlannedOccurrenceRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedOccurrence, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+occurrenceColumns+" FROM planned_occurrences WHERE id = $1", id)
	return scanOccurrence(row)
}

func (r *PlannedOccurrenceRepository) ListForItem(ctx context.Context, plannedItemID uuid.UUID) ([]entities.PlannedOccurrence, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+occurrenceColumns+" FROM planned_occurrences WHERE planned_item_id = $1", plannedItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.PlannedOccurrence
	for rows.Next() {
		o, err := scanOccurrenceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *PlannedOccurrenceRepository) ListForAccount(ctx context.Context, accountID uuid.UUID, dateFrom, dateTo *civil.Date) ([]entities.PlannedOccurrence, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT po.id, po.planned_item_id, po.due_date, po.expected_amount, po.status, po.reconciled_operation_id, po.reconciled_amount
		FROM planned_occurrences po
		JOIN planned_items pi ON po.planned_item_id = pi.id
		WHERE pi.account_id = $1`)
	args := []any{accountID}
	if dateFrom != nil {
		args = append(args, *dateFrom)
		sb.WriteString(fmt.Sprintf(" AND po.due_date >= $%d", len(args)))
	}
	if dateTo != nil {
		args = append(args, *dateTo)
		sb.WriteString(fmt.Sprintf(" AND po.due_date <= $%d", len(args)))
	}
	sb.WriteString(" ORDER BY po.due_date")

	rows, err := r.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.PlannedOccurrence
	for rows.Next() {
		o, err := scanOccurrenceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *PlannedOccurrenceRepository) GetByItemAndDate(ctx context.Context, plannedItemID uuid.UUID, dueDate civil.Date) (*entities.PlannedOccurrence, error) {
	row := r.pool.QueryRow(ctx,
		"SELECT "+occurrenceColumns+" FROM planned_occurrences WHERE planned_item_id = $1 AND due_date = $2",
		plannedItemID, dueDate,
	)
	return scanOccurrence(row)
}

func (r *PlannedOccurrenceRepository) Add(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error) {
	result, err := r.AddMany(ctx, []entities.PlannedOccurrence{*occ})
	if err != nil {
		return nil, err
	}
	return &result[0], nil
}

func (r *PlannedOccurrenceRepository) AddMany(ctx context.Context, occs []entities.PlannedOccurrence) ([]entities.PlannedOccurrence, error) {
	out := make([]entities.PlannedOccurrence, 0, len(occs))
	for _, occ := range occs {
		row := r.pool.QueryRow(ctx,
			`INSERT INTO planned_occurrences (id, planned_item_id, due_date, expected_amount, status, reconciled_operation_id, reconciled_amount)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING `+occurrenceColumns,
			occ.ID, occ.PlannedItemID, occ.DueDate, occ.ExpectedAmount, occ.Status, occ.ReconciledOperationID, occ.ReconciledAmount,
		)
		saved, err := scanOccurrence(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *saved)
	}
	return out, nil
}

func (r *PlannedOccurrenceRepository) Update(ctx context.Context, occ *entities.PlannedOccurrence) (*entities.PlannedOccurrence, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE planned_occurrences SET status = $2, reconciled_operation_id = $3, reconciled_amount = $4
		 WHERE id = $1
		 RETURNING `+occurrenceColumns,
		occ.ID, occ.Status, occ.ReconciledOperationID, occ.ReconciledAmount,
	)
	return scanOccurrence(row)
}
