package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"bailleul.dev/budget-server/internal/domain/entities"
)

type PlannedItemRepository struct{ pool *pgxpool.Pool }

func NewPlannedItemRepository(pool *pgxpool.Pool) *PlannedItemRepository {
	return &PlannedItemRepository{pool: pool}
}

const plannedItemColumns = "id, account_id, kind, label, match_mode, amount, start_date, recurrence_type, occurrence_count, interval_weeks, day_of_month, active, notes, category_id, created_at, updated_at"

func scanPlannedItem(row pgx.Row) (*entities.PlannedItem, error) {
	var i entities.PlannedItem
	err := row.Scan(&i.ID, &i.AccountID, &i.Kind, &i.Label, &i.MatchMode, &i.Amount, &i.StartDate, &i.RecurrenceType, &i.OccurrenceCount, &i.IntervalWeeks, &i.DayOfMonth, &i.Active, &i.Notes, &i.CategoryID, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func scanPlannedItemRow(rows pgx.Rows) (entities.PlannedItem, error) {
	var i entities.PlannedItem
	err := rows.Scan(&i.ID, &i.AccountID, &i.Kind, &i.Label, &i.MatchMode, &i.Amount, &i.StartDate, &i.RecurrenceType, &i.OccurrenceCount, &i.IntervalWeeks, &i.DayOfMonth, &i.Active, &i.Notes, &i.CategoryID, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

func (r *PlannedItemRepository) GetByID(ctx context.Context, id uuid.UUID) (*entities.PlannedItem, error) {
	row := r.pool.QueryRow(ctx, "SELECT "+plannedItemColumns+" FROM planned_items WHERE id = $1", id)
	return scanPlannedItem(row)
}

func (r *PlannedItemRepository) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]entities.PlannedItem, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+plannedItemColumns+" FROM planned_items WHERE account_id = $1", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.PlannedItem
	for rows.Next() {
		i, err := scanPlannedItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *PlannedItemRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entities.PlannedItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+plannedItemColumnsAliased()+` FROM planned_items pi
		 JOIN accounts a ON pi.account_id = a.id
		 WHERE a.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []entities.PlannedItem
	for rows.Next() {
		i, err := scanPlannedItemRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func plannedItemColumnsAliased() string {
	return "pi.id, pi.account_id, pi.kind, pi.label, pi.match_mode, pi.amount, pi.start_date, pi.recurrence_type, pi.occurrence_count, pi.interval_weeks, pi.day_of_month, pi.active, pi.notes, pi.category_id, pi.created_at, pi.updated_at"
}

func (r *PlannedItemRepository) Add(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO planned_items (id, account_id, kind, label, match_mode, amount, start_date, recurrence_type, occurrence_count, interval_weeks, day_of_month, active, notes, category_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+plannedItemColumns,
		item.ID, item.AccountID, item.Kind, item.Label, item.MatchMode, item.Amount, item.StartDate, item.RecurrenceType, item.OccurrenceCount, item.IntervalWeeks, item.DayOfMonth, item.Active, item.Notes, item.CategoryID,
	)
	return scanPlannedItem(row)
}

func (r *PlannedItemRepository) Update(ctx context.Context, item *entities.PlannedItem) (*entities.PlannedItem, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE planned_items SET kind = $2, label = $3, match_mode = $4, amount = $5, start_date = $6, recurrence_type = $7,
		   occurrence_count = $8, interval_weeks = $9, day_of_month = $10, active = $11, notes = $12, category_id = $13
		 WHERE id = $1
		 RETURNING `+plannedItemColumns,
		item.ID, item.Kind, item.Label, item.MatchMode, item.Amount, item.StartDate, item.RecurrenceType, item.OccurrenceCount, item.IntervalWeeks, item.DayOfMonth, item.Active, item.Notes, item.CategoryID,
	)
	return scanPlannedItem(row)
}
