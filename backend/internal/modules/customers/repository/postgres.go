// Package repository is the Postgres access path of the customers resource. The
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
	// ErrPhoneTaken is returned when the business already has a live record
	// with the phone.
	ErrPhoneTaken = errors.New("phone already used by another customer")
	// ErrIdentityLinked is returned when the identity is already linked to
	// another record of the business.
	ErrIdentityLinked = errors.New("identity already linked in the organization")
)

// Postgres runs the customers queries, alone or in one transaction.
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

// MapErr maps pgx.ErrNoRows to ErrNotFound, the phone index to
// ErrPhoneTaken and the identity link index to ErrIdentityLinked.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "uq_customers_phone":
			return ErrPhoneTaken
		case "uq_customer_identity_links_identity", "uq_customer_identity_links_customer":
			return ErrIdentityLinked
		}
	}
	return err
}
