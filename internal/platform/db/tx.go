package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// InTx runs fn in a transaction: commit if it returns nil, roll back if it
// returns an error or panics (the panic is re-raised). Everything that changes
// inventory, and every write with its audit_log row, goes through a transaction
// like this (AGENTS.md).
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *dbgen.Queries) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// The caller's context may be cancelled already, and the rollback must still happen.
	rollback := func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }
	defer func() {
		if r := recover(); r != nil {
			rollback()
			panic(r)
		}
	}()

	if err := fn(dbgen.New(tx)); err != nil {
		rollback()
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
