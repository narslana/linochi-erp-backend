//go:build integration

package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

func newPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newStore(t *testing.T) (*db.Store, *pgxpool.Pool) {
	t.Helper()
	pool := newPool(t, migrated(t))
	return db.NewStore(pool), pool
}

const issuer = "https://auth.lab.internal"

func login(subject, email, name string) db.UserLogin {
	return db.UserLogin{Issuer: issuer, Subject: subject, Email: email, DisplayName: name}
}

func TestNFK28_UpsertUserOnLogin(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)

	first, err := store.UpsertUserOnLogin(ctx, login("sub-1", "anna@example.se", "Anna"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.LastLoginAt == nil || first.ArchivedAt != nil {
		t.Fatalf("new user: version=%d last_login=%v archived=%v", first.Version, first.LastLoginAt, first.ArchivedAt)
	}
	if first.ID.Version() != 7 {
		t.Errorf("id %s is a version %d UUID, want 7", first.ID, first.ID.Version())
	}

	time.Sleep(20 * time.Millisecond)
	second, err := store.UpsertUserOnLogin(ctx, login("sub-1", "anna.s@example.se", "Anna S"))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("the same issuer and subject gave a new id: %s then %s", first.ID, second.ID)
	}
	if second.Email != "anna.s@example.se" || second.DisplayName != "Anna S" {
		t.Errorf("profile was not refreshed: %q %q", second.Email, second.DisplayName)
	}
	if second.Version != 2 {
		t.Errorf("version = %d, want 2 after the second login", second.Version)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("created_at changed")
	}
	if !second.LastLoginAt.After(*first.LastLoginAt) {
		t.Error("last_login_at was not moved forward")
	}
}

func TestNFK28_UsersAreUniquePerIssuerAndSubject(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)

	a, err := store.UpsertUserOnLogin(ctx, db.UserLogin{Issuer: "https://a.example", Subject: "same", Email: "a@example.se"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.UpsertUserOnLogin(ctx, db.UserLogin{Issuer: "https://b.example", Subject: "same", Email: "b@example.se"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Error("the same subject at two issuers must be two users")
	}
}

func TestNFK28_GetUserNotFound(t *testing.T) {
	store, _ := newStore(t)
	_, err := store.GetUser(context.Background(), uuid.Must(uuid.NewV7()))
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("err = %v, want db.ErrNotFound", err)
	}
}

func TestNFK28_ArchiveWithStaleVersionIsRejected(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	u, err := store.UpsertUserOnLogin(ctx, login("sub-1", "a@example.se", "A"))
	if err != nil {
		t.Fatal(err)
	}

	archived, err := store.ArchiveUser(ctx, u.ID, u.Version)
	if err != nil {
		t.Fatalf("archive with the current version: %v", err)
	}
	if archived.Version != u.Version+1 || archived.ArchivedAt == nil {
		t.Fatalf("after archive: version=%d archived_at=%v", archived.Version, archived.ArchivedAt)
	}

	// Someone else already changed the row: the same, now stale, version is rejected.
	if _, err := store.ArchiveUser(ctx, u.ID, u.Version); !errors.Is(err, db.ErrStaleVersion) {
		t.Fatalf("stale archive: err = %v, want db.ErrStaleVersion", err)
	}
	if _, err := store.ArchiveUser(ctx, uuid.Must(uuid.NewV7()), 1); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("archive of a missing user: err = %v, want db.ErrNotFound", err)
	}
}

// NFK-09: data is kept forever, archived records are hidden but not deleted.
func TestNFK09_ArchivedUsersAreKept(t *testing.T) {
	ctx := context.Background()
	store, pool := newStore(t)
	u, err := store.UpsertUserOnLogin(ctx, login("sub-1", "a@example.se", "A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveUser(ctx, u.ID, u.Version); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetUser(ctx, u.ID)
	if err != nil || got.ArchivedAt == nil {
		t.Fatalf("archived user: %+v, err %v", got, err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, err %v; archiving must not delete", n, err)
	}
}

func TestNFK28_DatabaseGeneratesUUIDv7WhenNoIDIsGiven(t *testing.T) {
	ctx := context.Background()
	_, pool := newStore(t)
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'e@example.se') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id.Version() != 7 {
		t.Errorf("the database default gave a version %d UUID, want 7", id.Version())
	}
}

func TestNFK28_InTxCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	store, pool := newStore(t)
	var id uuid.UUID
	err := db.InTx(ctx, pool, func(q *dbgen.Queries) error {
		u, err := q.UpsertUserOnLogin(ctx, dbgen.UpsertUserOnLoginParams{
			ID: uuid.Must(uuid.NewV7()), OidcIssuer: issuer, OidcSubject: "tx", Email: "tx@example.se"})
		id = u.ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUser(ctx, id); err != nil {
		t.Fatalf("committed user not found: %v", err)
	}
}

func TestNFK28_InTxRollsBackOnErrorAndOnPanic(t *testing.T) {
	ctx := context.Background()
	store, pool := newStore(t)
	insert := func(q *dbgen.Queries, subject string) uuid.UUID {
		t.Helper()
		u, err := q.UpsertUserOnLogin(ctx, dbgen.UpsertUserOnLoginParams{
			ID: uuid.Must(uuid.NewV7()), OidcIssuer: issuer, OidcSubject: subject, Email: subject + "@example.se"})
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	boom := errors.New("boom")

	var errID uuid.UUID
	err := db.InTx(ctx, pool, func(q *dbgen.Queries) error {
		errID = insert(q, "rolled-back-by-error")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx returned %v, want the callback's error", err)
	}
	if _, err := store.GetUser(ctx, errID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("a user written before an error survived: %v", err)
	}

	var panicID uuid.UUID
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("InTx swallowed the panic")
			}
		}()
		_ = db.InTx(ctx, pool, func(q *dbgen.Queries) error {
			panicID = insert(q, "rolled-back-by-panic")
			panic("kaboom")
		})
	}()
	if _, err := store.GetUser(ctx, panicID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("a user written before a panic survived: %v", err)
	}
}
