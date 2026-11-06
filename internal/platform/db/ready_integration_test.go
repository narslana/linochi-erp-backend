//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

func TestNFK02_ReadinessIsOKWhenMigrated(t *testing.T) {
	pool := newPool(t, migrated(t))
	if err := db.ReadinessCheck(pool)(context.Background()); err != nil {
		t.Fatalf("ready database reported: %v", err)
	}
}

func TestNFK02_ReadinessIsRedWhenTheSchemaIsBehind(t *testing.T) {
	// A database with no migrations applied: the binary is newer than the schema.
	pool := newPool(t, server.NewDatabase(t))
	if err := db.ReadinessCheck(pool)(context.Background()); err == nil {
		t.Fatal("expected an error for an unmigrated database")
	}
}

// A rollback to the previous image runs against a schema that is ahead of it.
// The release skill requires migrations to be backwards compatible, so that
// pod must stay ready.
func TestNFK02_ReadinessIsOKWhenTheSchemaIsAheadOfTheBinary(t *testing.T) {
	ctx := context.Background()
	dsn := migrated(t)
	conn := connect(t, dsn)
	if _, err := conn.Exec(ctx, `INSERT INTO goose_db_version (version_id, is_applied) VALUES (99991231235959, true)`); err != nil {
		t.Fatal(err)
	}
	if err := db.ReadinessCheck(newPool(t, dsn))(ctx); err != nil {
		t.Fatalf("a schema ahead of the binary must be accepted: %v", err)
	}
}

func TestNFK02_ReadinessIsRedWithoutDatabase(t *testing.T) {
	// Nothing listens on this port. pgxpool connects lazily, so the pool
	// itself is created fine, exactly as when the database is down at start-up.
	pool, err := pgxpool.New(context.Background(), "postgres://linochi:x@127.0.0.1:1/none?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("a pool must be creatable while the database is down: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.ReadinessCheck(pool)(ctx); err == nil {
		t.Fatal("expected an error without a database")
	}
}
