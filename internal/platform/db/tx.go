package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

// Tx is an open transaction. Code that must write in the same transaction as a
// change, such as audit.Recorder, takes a *Tx and not the pool, so that it
// cannot be called outside one.
type Tx struct {
	tx          pgx.Tx
	q           *dbgen.Queries
	afterCommit []func()
}

// Queries returns the sqlc queries bound to this transaction.
func (t *Tx) Queries() *dbgen.Queries { return t.q }

// AfterCommit registers fn to run once, after a successful commit, in the order
// registered. It never runs when the transaction is rolled back. Use it for what
// must happen only if the change is really saved, such as the audit log line to
// Loki. fn must not fail the request, so it has no error result.
func (t *Tx) AfterCommit(fn func()) { t.afterCommit = append(t.afterCommit, fn) }

// InTx runs fn in a transaction: commit if it returns nil, roll back if it
// returns an error or panics (the panic is re-raised). Everything that changes
// inventory, and every write with its audit_log row, goes through a transaction
// like this (AGENTS.md).
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx *Tx) error) (err error) {
	pgxTx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// The caller's context may be cancelled already, and the rollback must still happen.
	rollback := func() { _ = pgxTx.Rollback(context.WithoutCancel(ctx)) }
	defer func() {
		if r := recover(); r != nil {
			rollback()
			panic(r)
		}
	}()

	tx := &Tx{tx: pgxTx, q: dbgen.New(pgxTx)}
	if err := fn(tx); err != nil {
		rollback()
		return err
	}
	if err := pgxTx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	for _, hook := range tx.afterCommit {
		hook()
	}
	return nil
}
