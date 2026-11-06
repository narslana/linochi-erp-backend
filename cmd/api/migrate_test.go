package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func envWith(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func migrateCmd(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), append([]string{"api", "migrate"}, args...), envWith(env), &out, &errOut)
	return code, out.String(), errOut.String()
}

const dsn = "postgres://linochi:s3cr3t-pw@127.0.0.1:1/linochi_erp?sslmode=disable&connect_timeout=1"

func TestNFK28_MigrateWithoutActionPrintsUsage(t *testing.T) {
	code, _, stderr := migrateCmd(t, map[string]string{"MIGRATE_DATABASE_URL": dsn})
	if code != 2 || !strings.Contains(stderr, "usage: api migrate up|down|status") {
		t.Fatalf("code=%d stderr=%q, want exit 2 and the usage line", code, stderr)
	}
}

func TestNFK28_MigrateRejectsAnUnknownAction(t *testing.T) {
	code, _, stderr := migrateCmd(t, map[string]string{"MIGRATE_DATABASE_URL": dsn}, "sideways")
	if code != 2 || !strings.Contains(stderr, "sideways") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming the action", code, stderr)
	}
}

func TestNFK28_MigrateNeedsMigrateDatabaseURL(t *testing.T) {
	code, _, stderr := migrateCmd(t, map[string]string{}, "up")
	if code != 2 || !strings.Contains(stderr, "MIGRATE_DATABASE_URL") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming MIGRATE_DATABASE_URL", code, stderr)
	}
}

// The app role cannot change the schema. A migration that quietly fell back to
// it would fail later and confusingly, so there is no fallback.
func TestNFK28_MigrateDoesNotFallBackToTheAppURL(t *testing.T) {
	code, _, stderr := migrateCmd(t, map[string]string{"DATABASE_URL": dsn}, "up")
	if code != 2 || !strings.Contains(stderr, "MIGRATE_DATABASE_URL") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming MIGRATE_DATABASE_URL", code, stderr)
	}
}

// A Job that cannot reach the database must fail loudly and must not print the password.
func TestNFK28_MigrateFailsWithoutDatabaseAndKeepsThePasswordOut(t *testing.T) {
	code, stdout, stderr := migrateCmd(t, map[string]string{"MIGRATE_DATABASE_URL": dsn}, "up")
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
	if strings.Contains(stdout+stderr, "s3cr3t-pw") {
		t.Errorf("the password leaked: %q %q", stdout, stderr)
	}
}
