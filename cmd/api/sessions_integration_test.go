//go:build integration

package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

type sessionsFixture struct {
	env      map[string]string
	pool     *pgxpool.Pool
	users    *auth.UserStore
	sessions *auth.SessionStore
}

func newSessionsFixture(t *testing.T) *sessionsFixture {
	t.Helper()
	owner := pgServer.NewMigratedDatabase(t)
	appURL := testdb.AppURL(owner)
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{URL: appURL, MaxConns: 4, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rec, err := audit.NewRecorder(slog.New(slog.DiscardHandler), metrics)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionStore(pool, rec, metrics)
	if err != nil {
		t.Fatal(err)
	}
	return &sessionsFixture{
		env: map[string]string{"DATABASE_URL": appURL}, pool: pool,
		users: auth.NewUserStore(pool, rec), sessions: sessions,
	}
}

func (f *sessionsFixture) user(t *testing.T, subject, email string) uuid.UUID {
	t.Helper()
	u, err := f.users.UpsertOnLogin(context.Background(), auth.UserLogin{
		Issuer: "https://auth.lab.internal", Subject: subject, Email: email, DisplayName: subject,
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func (f *sessionsFixture) login(t *testing.T, id uuid.UUID, n int) []string {
	t.Helper()
	var tokens []string
	for range n {
		token, _, err := f.sessions.Create(context.Background(), id, []string{"admin"})
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func (f *sessionsFixture) count(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions WHERE user_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNFK11_RevokeUserEndsEverySessionOfTheUserAndNoOthers(t *testing.T) {
	f := newSessionsFixture(t)
	anna := f.user(t, "anna", "Anna.Svensson@example.se")
	bo := f.user(t, "bo", "bo@example.se")
	f.login(t, anna, 3)
	f.login(t, bo, 1)

	// The address is matched without regard to case: nobody types it as stored.
	code, out, errOut := sessionsCmd(t, f.env, "revoke-user", "anna.svensson@EXAMPLE.se")
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "ended 3 sessions") || !strings.Contains(out, anna.String()) {
		t.Errorf("stdout = %q, want the count and the user id", out)
	}
	if n := f.count(t, anna); n != 0 {
		t.Errorf("%d of Anna's sessions are left", n)
	}
	if n := f.count(t, bo); n != 1 {
		t.Errorf("Bo has %d sessions, want 1: another user's login was ended", n)
	}
	// Personal data stays out of what the command prints (AGENTS.md): the id is
	// enough to find the person, and the address is not echoed.
	if strings.Contains(out+errOut, "example.se") {
		t.Errorf("the e-mail address was printed: %q %q", out, errOut)
	}

	rows, err := dbgen.New(f.pool).ListAuditEntriesForEntity(context.Background(),
		dbgen.ListAuditEntriesForEntityParams{EntityType: "user", EntityID: anna})
	if err != nil {
		t.Fatal(err)
	}
	var revoked *dbgen.AuditLog
	for i := range rows {
		if rows[i].Action == "sessions_revoked" {
			revoked = &rows[i]
		}
	}
	if revoked == nil {
		t.Fatal("no sessions_revoked entry: the command changed data without a trace")
	}
	if revoked.ActorType != "system" || revoked.ActorLabel != "cli-revoke-user" {
		t.Errorf("actor = %s/%q, want system/cli-revoke-user", revoked.ActorType, revoked.ActorLabel)
	}
}

func TestNFK11_RevokeUserAcceptsAUserID(t *testing.T) {
	f := newSessionsFixture(t)
	anna := f.user(t, "anna", "anna@example.se")
	f.login(t, anna, 2)

	code, out, errOut := sessionsCmd(t, f.env, "revoke-user", anna.String())
	if code != 0 || !strings.Contains(out, "ended 2 sessions") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if n := f.count(t, anna); n != 0 {
		t.Errorf("%d sessions left", n)
	}
}

func TestNFK11_RevokeUserForAUserWithNoSessionsSucceedsAndSaysSo(t *testing.T) {
	f := newSessionsFixture(t)
	f.user(t, "anna", "anna@example.se")
	code, out, errOut := sessionsCmd(t, f.env, "revoke-user", "anna@example.se")
	if code != 0 || !strings.Contains(out, "ended 0 sessions") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestNFK11_RevokeUserRefusesAnUnknownUserAndTouchesNothing(t *testing.T) {
	f := newSessionsFixture(t)
	anna := f.user(t, "anna", "anna@example.se")
	f.login(t, anna, 1)

	for _, who := range []string{"nobody@example.se", uuid.NewString()} {
		code, out, errOut := sessionsCmd(t, f.env, "revoke-user", who)
		if code != 1 || !strings.Contains(errOut, "no such user") {
			t.Errorf("%s: code=%d stdout=%q stderr=%q, want exit 1 and \"no such user\"", who, code, out, errOut)
		}
	}
	if n := f.count(t, anna); n != 1 {
		t.Errorf("a refused command ended %d sessions", 1-n)
	}
}

func TestNFK11_RevokeUserRefusesAnAmbiguousAddressAndListsTheIDs(t *testing.T) {
	f := newSessionsFixture(t)
	// Two Authelia accounts with the same address: the schema allows it, since
	// a user is (issuer, subject). Guessing which one is meant is not fine.
	a := f.user(t, "anna-1", "anna@example.se")
	b := f.user(t, "anna-2", "ANNA@example.se")
	f.login(t, a, 1)
	f.login(t, b, 1)

	code, out, errOut := sessionsCmd(t, f.env, "revoke-user", "anna@example.se")
	if code != 1 {
		t.Fatalf("code=%d stdout=%q, want exit 1", code, out)
	}
	for _, id := range []uuid.UUID{a, b} {
		if !strings.Contains(errOut, id.String()) {
			t.Errorf("stderr %q does not list %s, so the caller cannot pick one", errOut, id)
		}
	}
	if f.count(t, a)+f.count(t, b) != 2 {
		t.Error("an ambiguous command ended sessions anyway")
	}
	// And the ids do work.
	if code, out, errOut := sessionsCmd(t, f.env, "revoke-user", a.String()); code != 0 {
		t.Fatalf("by id: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if f.count(t, a) != 0 || f.count(t, b) != 1 {
		t.Errorf("sessions after revoking only A: A=%d B=%d, want 0 and 1", f.count(t, a), f.count(t, b))
	}
}

func TestNFK11_RevokeUserWorksAsTheAppRoleAndNeedsNothingMore(t *testing.T) {
	// The fixture's URL is the app role's, so every test above already proves
	// it. This one says so out loud: if a later migration narrows the grants
	// the command depends on, this is where it shows.
	f := newSessionsFixture(t)
	var role string
	if err := f.pool.QueryRow(context.Background(), `SELECT current_user`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != testdb.AppRole {
		t.Fatalf("fixture connects as %q, want the app role %q", role, testdb.AppRole)
	}
}

// The cleanup is wired into the running app, not just a method on the store.
func TestNFK11_TheRunningAppDeletesEndedSessionsAndKeepsLiveOnes(t *testing.T) {
	ownerURL := pgServer.NewMigratedDatabase(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	var userID uuid.UUID
	if err := conn.QueryRow(ctx, `INSERT INTO users (oidc_issuer, oidc_subject, email) VALUES ('https://auth.lab.internal', 'anna', 'anna@example.se') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	// Three sessions: one past its 7 days, one idle for 13 hours, one in use.
	insert := func(created, lastSeen, expires string) uuid.UUID {
		t.Helper()
		token, hash, err := auth.NewToken()
		_ = token
		if err != nil {
			t.Fatal(err)
		}
		var id uuid.UUID
		err = conn.QueryRow(ctx, `INSERT INTO sessions (user_id, token_hash, roles, created_at, last_seen_at, expires_at)
			VALUES ($1, $2, '{admin}', now() - $3::interval, now() - $4::interval, now() + $5::interval) RETURNING id`,
			userID, hash, created, lastSeen, expires).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	absolute := insert("8 days", "1 hour", "-1 day")
	idle := insert("1 day", "13 hours", "6 days")
	live := insert("1 hour", "1 minute", "7 days")

	_, stop := start(t, testdb.AppURL(ownerURL))
	t.Cleanup(func() { stop() })

	exists := func(id uuid.UUID) bool {
		var n int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && (exists(absolute) || exists(idle)) {
		time.Sleep(100 * time.Millisecond)
	}
	if exists(absolute) {
		t.Error("a session past its 7 days is still there: the app does not run the cleanup")
	}
	if exists(idle) {
		t.Error("a session idle for 13 hours is still there")
	}
	if !exists(live) {
		t.Error("the cleanup deleted a session that is in use")
	}
}
