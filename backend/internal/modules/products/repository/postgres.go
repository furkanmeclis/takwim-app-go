// Package repository is the Postgres access path of the products resource.
// The sqlc queries take the organization id explicitly and filter by it,
// so a uuid from another organization reads as not found.
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
	// ErrCodeTaken is returned when a product or variant already uses the SKU.
	ErrCodeTaken = errors.New("stock code already used")
	// ErrVariantNameTaken is returned when the product has a variant with the name.
	ErrVariantNameTaken = errors.New("variant name already used")
	// ErrNegativeStock is returned when a write would leave stock below zero.
	ErrNegativeStock = errors.New("stock cannot go below zero")
)

// Postgres runs the products queries, alone or in one transaction.
type Postgres struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgres builds the repository. pool may be nil (no transactions:
// InTx runs on q directly).
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

// MapErr maps pgx.ErrNoRows to ErrNotFound, the code indexes to
// ErrCodeTaken / ErrVariantNameTaken and the stock checks to
// ErrNegativeStock.
func MapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			switch pgErr.ConstraintName {
			case "uq_products_sku", "uq_product_variants_sku":
				return ErrCodeTaken
			case "uq_product_variants_name":
				return ErrVariantNameTaken
			}
		case "23514":
			switch pgErr.ConstraintName {
			case "ck_stock_levels_on_hand", "stock_movements_balance_after_check":
				return ErrNegativeStock
			}
		}
	}
	return err
}
