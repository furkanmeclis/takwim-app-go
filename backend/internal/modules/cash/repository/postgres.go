// Package repository is the Postgres access path of the cash module. The
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
	// ErrNameTaken is returned when the branch already has an account with the name.
	ErrNameTaken = errors.New("name already used in the branch")
	// ErrAlreadyReversed is returned when a movement was reversed before.
	ErrAlreadyReversed = errors.New("movement already reversed")
	// ErrDayClosed is returned when a movement falls on a closed day.
	ErrDayClosed = errors.New("day closed")
	// ErrDayAlreadyClosed is returned when the day has a closing already.
	ErrDayAlreadyClosed = errors.New("day already closed")
	// ErrIdempotencyKey is returned when the idempotency key was used before.
	ErrIdempotencyKey = errors.New("idempotency key used")
)

// Postgres runs the cash queries, alone or in one transaction.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil (no transactions:
// InTx runs on q directly, as in unit tests over one connection).
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

// MapErr maps pgx.ErrNoRows and the constraint violations of the cash
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
		case "uq_cash_accounts_branch_name":
			return ErrNameTaken
		case "uq_cash_movements_reverses":
			return ErrAlreadyReversed
		case "cash_movements_day_closed":
			return ErrDayClosed
		case "uq_cash_closings_day":
			return ErrDayAlreadyClosed
		case "uq_cash_movements_idempotency":
			return ErrIdempotencyKey
		}
	}
	return err
}
