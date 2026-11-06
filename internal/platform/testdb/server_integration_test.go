//go:build integration

package testdb_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

var server *testdb.Server

func TestMain(m *testing.M) {
	os.Exit(func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := testdb.StartServer(ctx)
		if err != nil {
			_, _ = os.Stderr.WriteString("start postgres: " + err.Error() + "\n")
			return 1
		}
		server = s
		defer func() { _ = s.Terminate(context.Background()) }()
		return m.Run()
	}())
}

func exec(t *testing.T, dsn, sql string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func tableExists(t *testing.T, dsn, table string) bool {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// Every test gets a database of its own, so tests cannot see each other's data
// and can run against one shared container (fast, also on the arm64 runner).
func TestNFK28_NewDatabaseIsolatesTests(t *testing.T) {
	a := server.NewDatabase(t)
	b := server.NewDatabase(t)
	if a == b {
		t.Fatal("two calls returned the same database")
	}
	exec(t, a, `CREATE TABLE only_in_a (id int)`)
	if !tableExists(t, a, "only_in_a") {
		t.Error("table missing in the database that created it")
	}
	if tableExists(t, b, "only_in_a") {
		t.Error("table leaked into another test's database")
	}
}

func TestNFK28_NewDatabaseIsDroppedAfterTheTest(t *testing.T) {
	var dsn string
	t.Run("create", func(t *testing.T) { dsn = server.NewDatabase(t) })
	// The subtest's cleanup has run: the database is gone.
	ctx := context.Background()
	if conn, err := pgx.Connect(ctx, dsn); err == nil {
		_ = conn.Close(ctx)
		t.Error("the database still exists after its test finished")
	}
}

func currentUser(t *testing.T, dsn string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	var u string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&u); err != nil {
		t.Fatal(err)
	}
	return u
}

// The test databases mirror production: an owner role that owns the schema and
// runs migrations, and a restricted app role that the running app uses.
func TestNFK28_NewDatabaseIsOwnedByTheOwnerRoleAndAppURLIsTheRestrictedRole(t *testing.T) {
	owner := server.NewDatabase(t)
	if got := currentUser(t, owner); got != "linochi_erp_owner" {
		t.Errorf("the returned URL connects as %q, want linochi_erp_owner", got)
	}
	app := testdb.AppURL(owner)
	if got := currentUser(t, app); got != "linochi_erp_app" {
		t.Errorf("AppURL connects as %q, want linochi_erp_app", got)
	}

	exec(t, owner, `CREATE TABLE made_by_owner (id int)`) // the owner may change the schema
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE made_by_app (id int)`); err == nil {
		t.Error("the app role must not be able to create tables")
	}
	if _, err := conn.Exec(ctx, `DROP TABLE made_by_owner`); err == nil {
		t.Error("the app role must not be able to drop the owner's tables")
	}
}
