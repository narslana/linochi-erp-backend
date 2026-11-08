package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func sessionsCmd(t *testing.T, env map[string]string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), append([]string{"api", "sessions"}, args...), envWith(env), &out, &errOut)
	return code, out.String(), errOut.String()
}

// The usage errors are found before the database is touched, so these run
// without one and with a DATABASE_URL that could not connect.
func TestNFK11_SessionsCommandUsageErrors(t *testing.T) {
	env := map[string]string{"DATABASE_URL": dsn}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no action", nil, "usage: api sessions revoke-user <email|user-id>"},
		{"unknown action", []string{"sideways"}, "sideways"},
		{"revoke-user without who", []string{"revoke-user"}, "usage: api sessions revoke-user <email|user-id>"},
		{"revoke-user with two", []string{"revoke-user", "a@example.se", "b@example.se"}, "usage: api sessions revoke-user <email|user-id>"},
		{"revoke-user with an empty argument", []string{"revoke-user", "  "}, "usage: api sessions revoke-user <email|user-id>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := sessionsCmd(t, env, tc.args...)
			if code != 2 || !strings.Contains(errOut, tc.want) {
				t.Fatalf("code=%d stderr=%q, want exit 2 and %q", code, errOut, tc.want)
			}
			if out != "" {
				t.Errorf("stdout = %q, want nothing on a usage error", out)
			}
		})
	}
}

func TestNFK11_SessionsCommandNeedsTheAppDatabaseURL(t *testing.T) {
	code, _, errOut := sessionsCmd(t, map[string]string{}, "revoke-user", "anna@example.se")
	if code != 2 || !strings.Contains(errOut, "DATABASE_URL") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming DATABASE_URL", code, errOut)
	}
	// The migration URL is the owner's. The command must not quietly use it:
	// it runs as the app role, which is all it needs.
	code, _, errOut = sessionsCmd(t, map[string]string{"MIGRATE_DATABASE_URL": dsn}, "revoke-user", "anna@example.se")
	if code != 2 || !strings.Contains(errOut, "DATABASE_URL") {
		t.Fatalf("with only MIGRATE_DATABASE_URL: code=%d stderr=%q, want exit 2", code, errOut)
	}
}

func TestNFK14_SessionsCommandNeverPrintsThePassword(t *testing.T) {
	// 127.0.0.1:1 refuses at once. Whatever the error says, the password in the
	// URL is not in it.
	code, out, errOut := sessionsCmd(t, map[string]string{"DATABASE_URL": dsn}, "revoke-user", "anna@example.se")
	if code == 0 {
		t.Fatal("revoke-user succeeded against a database that is not there")
	}
	if strings.Contains(out+errOut, "s3cr3t-pw") {
		t.Errorf("the password is in the output: %q", out+errOut)
	}
}
