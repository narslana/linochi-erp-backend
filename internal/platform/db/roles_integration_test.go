//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

func appConn(t *testing.T, ownerURL string) *pgx.Conn {
	t.Helper()
	return connect(t, testdb.AppURL(ownerURL))
}

func mustFailWith(t *testing.T, what string, conn *pgx.Conn, sql, want string) {
	t.Helper()
	_, err := conn.Exec(context.Background(), sql)
	if err == nil {
		t.Errorf("%s: expected an error, got none", what)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("%s: error %q does not contain %q", what, err, want)
	}
}

// The running app is not the owner. It reads and writes what it must, cannot
// delete the records NFK-09 says are kept forever, and cannot change the schema.
func TestNFK28_AppRoleHasTheRightsTheAppNeedsAndNoMore(t *testing.T) {
	ctx := context.Background()
	owner := migrated(t)
	app := appConn(t, owner)

	if _, err := app.Exec(ctx, `INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('i', 's', 'a@example.se')`); err != nil {
		t.Fatalf("insert as app: %v", err)
	}
	if _, err := app.Exec(ctx, `UPDATE users SET display_name = 'A' WHERE oidc_subject = 's'`); err != nil {
		t.Fatalf("update as app: %v", err)
	}
	var n int
	if err := app.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("select as app: n=%d err=%v", n, err)
	}
	// Readiness reads the schema version.
	if err := app.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(new(int64)); err != nil {
		t.Fatalf("the app must be able to read goose_db_version: %v", err)
	}

	mustFailWith(t, "DELETE FROM users", app, `DELETE FROM users`, "permission denied")
	mustFailWith(t, "TRUNCATE users", app, `TRUNCATE users`, "permission denied")
	mustFailWith(t, "CREATE TABLE", app, `CREATE TABLE sneaky (id int)`, "permission denied")
	mustFailWith(t, "DROP TABLE", app, `DROP TABLE users`, "must be owner")
	mustFailWith(t, "write to goose_db_version", app, `DELETE FROM goose_db_version`, "permission denied")
}
