//go:build integration

package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func tableNames(t *testing.T, conn *pgx.Conn) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(),
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func migrated(t *testing.T) string {
	t.Helper()
	dsn := server.NewDatabase(t)
	m, err := db.NewMigrator(dsn)
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	defer func() { _ = m.Close() }()
	if _, err := m.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return dsn
}

func TestNFK28_MigrationsUpDownUp(t *testing.T) {
	ctx := context.Background()
	dsn := server.NewDatabase(t)
	m, err := db.NewMigrator(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	conn := connect(t, dsn)

	applied, err := m.Up(ctx)
	if err != nil || len(applied) == 0 {
		t.Fatalf("Up applied %v, err %v", applied, err)
	}
	if got := tableNames(t, conn); !contains(got, "users") {
		t.Fatalf("after Up the tables are %v, want users among them", got)
	}
	latest, err := db.LatestVersion()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := m.Version(ctx); err != nil || v != latest {
		t.Fatalf("version after Up = %d (%v), want the latest %d", v, err, latest)
	}

	// Down, one step at a time, until nothing is left to roll back.
	for steps := 0; ; steps++ {
		if _, err := m.Down(ctx); err != nil {
			if !errors.Is(err, db.ErrNothingToRollBack) {
				t.Fatalf("Down: %v", err)
			}
			break
		}
		if steps > 50 {
			t.Fatal("Down never reached the start")
		}
	}
	if got := tableNames(t, conn); len(got) != 1 || got[0] != "goose_db_version" {
		t.Fatalf("after Down only goose_db_version should remain, have %v", got)
	}

	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if got := tableNames(t, conn); !contains(got, "users") {
		t.Fatalf("after the second Up the tables are %v", got)
	}
}

func TestNFK28_UpIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := migrated(t)
	m, err := db.NewMigrator(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if applied, err := m.Up(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("a second Up applied %v, err %v; want nothing", applied, err)
	}
}

// Every table, now and in the future, has the standard columns (AGENTS.md).
func TestNFK28_EveryTableHasStandardColumns(t *testing.T) {
	conn := connect(t, migrated(t))
	ctx := context.Background()

	var tables []string
	for _, n := range tableNames(t, conn) {
		if n != "goose_db_version" {
			tables = append(tables, n)
		}
	}
	if len(tables) == 0 {
		t.Fatal("no tables found: this test would pass vacuously")
	}

	type col struct{ typ, def string }
	for _, table := range tables {
		rows, err := conn.Query(ctx, `
			SELECT column_name, data_type, coalesce(column_default, '')
			FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1`, table)
		if err != nil {
			t.Fatal(err)
		}
		cols := map[string]col{}
		for rows.Next() {
			var name string
			var c col
			if err := rows.Scan(&name, &c.typ, &c.def); err != nil {
				t.Fatal(err)
			}
			cols[name] = c
		}
		rows.Close()

		want := map[string]string{
			"id": "uuid", "created_at": "timestamp with time zone",
			"updated_at": "timestamp with time zone", "version": "integer",
		}
		for name, typ := range want {
			c, ok := cols[name]
			switch {
			case !ok:
				t.Errorf("table %s has no column %s", table, name)
			case c.typ != typ:
				t.Errorf("table %s: %s is %s, want %s", table, name, c.typ, typ)
			}
		}
		if c := cols["id"]; !strings.Contains(c.def, "uuidv7()") {
			t.Errorf("table %s: id default is %q, want uuidv7() as a safety net", table, c.def)
		}
		if c := cols["version"]; !strings.HasPrefix(c.def, "1") {
			t.Errorf("table %s: version default is %q, want 1", table, c.def)
		}
	}
}

func TestNFK28_UpdatedAtIsSetByTrigger(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, migrated(t))

	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO users (oidc_issuer, oidc_subject, email, display_name)
		VALUES ('https://auth.example', 'sub-1', 'a@example.se', 'A') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// now() is the transaction time, so the update must be a transaction of its own.
	if _, err := conn.Exec(ctx, `SELECT pg_sleep(0.05)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `UPDATE users SET display_name = 'B' WHERE id = $1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	var created, updated bool
	if err := conn.QueryRow(ctx, `SELECT updated_at > created_at, created_at = (SELECT created_at FROM users WHERE id = $1::uuid) FROM users WHERE id = $1::uuid`, id).Scan(&updated, &created); err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Error("updated_at was not moved forward by the update")
	}
	if !created {
		t.Error("created_at changed")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
