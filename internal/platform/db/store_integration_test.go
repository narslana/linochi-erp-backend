//go:build integration

package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

// newPool opens a pool as the restricted app role, like the running app does.
func newPool(t *testing.T, ownerURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.AppURL(ownerURL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

const issuer = "https://auth.lab.internal"

func newUser(ctx context.Context, t *testing.T, q *dbgen.Queries, subject string) dbgen.User {
	t.Helper()
	u, err := q.InsertUserIfNew(ctx, dbgen.InsertUserIfNewParams{
		ID: uuid.Must(uuid.NewV7()), OidcIssuer: issuer, OidcSubject: subject, Email: subject + "@example.se"})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func userExists(ctx context.Context, t *testing.T, pool *pgxpool.Pool, id uuid.UUID) bool {
	t.Helper()
	_, err := dbgen.New(pool).GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func TestNFK28_InTxCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, migrated(t))
	var id uuid.UUID
	err := db.InTx(ctx, pool, func(tx *db.Tx) error {
		id = newUser(ctx, t, tx.Queries(), "tx").ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !userExists(ctx, t, pool, id) {
		t.Fatal("committed user not found")
	}
}

func TestNFK28_InTxRollsBackOnErrorAndOnPanic(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, migrated(t))
	boom := errors.New("boom")

	var errID uuid.UUID
	err := db.InTx(ctx, pool, func(tx *db.Tx) error {
		errID = newUser(ctx, t, tx.Queries(), "rolled-back-by-error").ID
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx returned %v, want the callback's error", err)
	}
	if userExists(ctx, t, pool, errID) {
		t.Error("a user written before an error survived")
	}

	var panicID uuid.UUID
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("InTx swallowed the panic")
			}
		}()
		_ = db.InTx(ctx, pool, func(tx *db.Tx) error {
			panicID = newUser(ctx, t, tx.Queries(), "rolled-back-by-panic").ID
			panic("kaboom")
		})
	}()
	if userExists(ctx, t, pool, panicID) {
		t.Error("a user written before a panic survived")
	}
}

// Things that must happen only if the change is really saved, such as the audit
// log line to Loki, are registered as hooks that run after the commit.
func TestNFK28_AfterCommitHooksRunOnlyAfterCommit(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, migrated(t))
	var order []string
	var visible bool

	err := db.InTx(ctx, pool, func(tx *db.Tx) error {
		u := newUser(ctx, t, tx.Queries(), "hooked")
		tx.AfterCommit(func() {
			order = append(order, "first")
			// The hook runs after the commit: another connection can see the row.
			visible = userExists(ctx, t, pool, u.ID)
		})
		tx.AfterCommit(func() { order = append(order, "second") })
		if len(order) != 0 {
			t.Error("a hook ran before the commit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("hooks ran %v, want first then second", order)
	}
	if !visible {
		t.Error("the committed row was not visible to the hook")
	}

	var ranOnError, ranOnPanic bool
	_ = db.InTx(ctx, pool, func(tx *db.Tx) error {
		tx.AfterCommit(func() { ranOnError = true })
		return errors.New("boom")
	})
	func() {
		defer func() { _ = recover() }()
		_ = db.InTx(ctx, pool, func(tx *db.Tx) error {
			tx.AfterCommit(func() { ranOnPanic = true })
			panic("kaboom")
		})
	}()
	if ranOnError || ranOnPanic {
		t.Errorf("hooks ran for a rolled-back transaction: error=%v panic=%v", ranOnError, ranOnPanic)
	}
}

func TestNFK28_DatabaseGeneratesUUIDv7WhenNoIDIsGiven(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, migrated(t))
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'e@example.se') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id.Version() != 7 {
		t.Errorf("the database default gave a version %d UUID, want 7", id.Version())
	}
}
