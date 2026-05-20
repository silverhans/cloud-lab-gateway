// Package pgxrepo implements storage ports with pgx and sqlc.
package pgxrepo

import (
	"context"
	"fmt"

	"github.com/cloud-lab-gateway/gateway/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UoW wraps pgx transactions for application use cases.
type UoW struct {
	pool *pgxpool.Pool
}

var _ ports.UnitOfWork = (*UoW)(nil)

// NewUoW creates a Postgres UnitOfWork backed by a pgx pool.
func NewUoW(pool *pgxpool.Pool) *UoW {
	return &UoW{pool: pool}
}

// WithTx executes fn inside a ReadCommitted transaction.
func (u *UoW) WithTx(ctx context.Context, fn func(ctx context.Context, tx ports.Tx) error) error {
	if u == nil || u.pool == nil {
		return fmt.Errorf("pgxrepo: nil pool")
	}
	return pgx.BeginTxFunc(ctx, u.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		return fn(ctx, pgxTx{tx: tx})
	})
}

type pgxTx struct {
	tx pgx.Tx
}

func (pgxTx) Private() {}

func txFromPort(tx ports.Tx) (pgx.Tx, error) {
	// Unsafe assert is acceptable here because UoW is the only constructor of Tx.
	pgxTx, ok := tx.(pgxTx)
	if !ok {
		return nil, fmt.Errorf("pgxrepo: unexpected tx type %T", tx)
	}
	return pgxTx.tx, nil
}
