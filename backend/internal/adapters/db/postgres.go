// Package db holds the pgx-backed implementations of the domain repository
// ports — mirrors backend/app/adapters/db/*.py. Each repository wraps the
// shared pool; no ORM, plain SQL, same table/column shapes as the existing
// Postgres schema (created by the Python backend's Alembic migrations —
// this Go server targets that same schema rather than re-deriving it).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}
