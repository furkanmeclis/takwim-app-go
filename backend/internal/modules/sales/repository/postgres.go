// Package repository is the Postgres access path of the sales module. The
// sqlc queries take the organization id explicitly and filter by it, so a
// uuid from another organization reads as not found.
package repository

import (
	"context"
	"errors"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound is returned for rows missing in the organization.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyReversed is returned when a line or entry was reversed before.
	ErrAlreadyReversed = errors.New("already reversed")
	// ErrLiveSale is returned when the item has a live sale line already.
	ErrLiveSale = errors.New("the item has a live sale line")
	// ErrIdempotencyKey is returned when the idempotency key was used before.
	ErrIdempotencyKey = errors.New("idempotency key used")
)

// Postgres runs the sales queries, alone or in one transaction.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil (InTx then runs on q).
func NewPostgres(pool *pgxpool.Pool, q *db.Queries) *Postgres {
	return &Postgres{pool: pool, q: q}
}

// Q returns the queries outside a transaction.
func (r *Postgres) Q() *db.Queries {
	return r.q
}

// InTx runs fn in one transaction (committed when fn returns nil).
func (r *Postgres) InTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if r.pool == nil {
		return fn(r.q)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MapErr maps pgx.ErrNoRows and the constraint violations of the sales
// tables to the repository errors.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "uq_sale_lines_reverses", "uq_sale_lines_replaces", "uq_ledger_entries_reverses":
			return ErrAlreadyReversed
		case "uq_sale_lines_accrual", "uq_ledger_entries_sale_line":
			return ErrLiveSale
		case "uq_ledger_entries_idempotency":
			return ErrIdempotencyKey
		}
	}
	return err
}
