//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// syncBuffer is a log buffer that two pods may write to at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testClock is a clock the test moves by hand, shared by every store in a test.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type sessionEnv struct {
	*env
	sessions *auth.SessionStore
	clock    *testClock
	reg      *prometheus.Registry
	// sessionLogs is what the session stores log, apart from env.logs which the
	// user store writes to: a user store and a session store run at the same
	// time in some tests.
	sessionLogs *syncBuffer
}

// newSessionEnv has one migrated database, the user store from newEnv and a
// session store on a pool of its own. A second store on a second pool is made
// with another().
func newSessionEnv(t *testing.T) *sessionEnv {
	t.Helper()
	e := newEnv(t)
	se := &sessionEnv{env: e, clock: &testClock{t: t0}, sessionLogs: &syncBuffer{}}
	se.sessions, se.reg = se.another(t)
	return se
}

// another is a second "pod": its own pool, recorder and metrics, the same database.
// Every pool is the app role, as in production.
func (se *sessionEnv) another(t *testing.T) (*auth.SessionStore, *prometheus.Registry) {
	t.Helper()
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	// A pool of its own, so that two pods really are two sets of connections.
	pool, err := db.Open(context.Background(), db.Options{URL: se.pool.Config().ConnString(), MaxConns: 4, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	rec, err := audit.NewRecorder(obs.NewLogger(se.sessionLogs, slog.LevelDebug), metrics)
	if err != nil {
		t.Fatal(err)
	}
	s, err := auth.NewSessionStore(pool, rec, metrics, auth.WithClock(se.clock.Now), auth.WithTracerProvider(tp))
	if err != nil {
		t.Fatal(err)
	}
	return s, metrics.Registry
}

func (se *sessionEnv) newUser(t *testing.T, subject string) dbgen.User {
	t.Helper()
	u, err := se.users.UpsertOnLogin(context.Background(), login(subject, subject+"@example.se", subject))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (se *sessionEnv) login(t *testing.T, userID uuid.UUID) (string, auth.Session) {
	t.Helper()
	token, s, err := se.sessions.Create(context.Background(), userID, []string{"admin"})
	if err != nil {
		t.Fatal(err)
	}
	return token, s
}

func (se *sessionEnv) countSessions(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := se.pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (se *sessionEnv) auditFor(t *testing.T, entityType string, id uuid.UUID) []dbgen.AuditLog {
	t.Helper()
	rows, err := dbgen.New(se.pool).ListAuditEntriesForEntity(context.Background(),
		dbgen.ListAuditEntriesForEntityParams{EntityType: entityType, EntityID: id})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func counter(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
	metric:
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if want, ok := labels[l.GetName()]; ok && want != l.GetValue() {
					continue metric
				}
			}
			return m.GetCounter().GetValue()
		}
	}
	return 0
}

func mustBeUnauthenticated(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("err = %v, want auth.ErrUnauthenticated", err)
	}
}

func TestNFK11_OnlyTokenHashIsStoredInTheDatabase(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)

	var hash []byte
	var rowText string
	if err := se.pool.QueryRow(context.Background(),
		`SELECT token_hash, sessions::text FROM sessions WHERE id = $1`, s.ID).Scan(&hash, &rowText); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte(token))
	if !bytes.Equal(hash, want[:]) {
		t.Error("token_hash is not the SHA-256 of the token")
	}
	if strings.Contains(rowText, token) {
		t.Error("the token itself is somewhere in the stored row")
	}
}

func TestNFK02_TwoInstancesShareASession(t *testing.T) {
	se := newSessionEnv(t)
	podB, _ := se.another(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)

	got, err := podB.Lookup(context.Background(), token)
	if err != nil {
		t.Fatalf("pod B does not know the session pod A created: %v", err)
	}
	if got.UserID != u.ID || len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Errorf("session on pod B = %+v", got)
	}

	if err := podB.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	_, err = se.sessions.Lookup(context.Background(), token)
	mustBeUnauthenticated(t, err)
}

func TestNFK11_LookupExpiresAfter12hIdle(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)

	se.clock.Advance(12*time.Hour - time.Second)
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Fatalf("one second before 12 hours idle: %v", err)
	}
	// That lookup was activity: the idle clock starts again from here.
	se.clock.Advance(12*time.Hour - time.Second)
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Fatalf("activity did not keep the session alive: %v", err)
	}
	se.clock.Advance(12 * time.Hour)
	_, err := se.sessions.Lookup(context.Background(), token)
	mustBeUnauthenticated(t, err)
}

func TestNFK11_LookupExpiresAfter7dAbsolute(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)

	// Used every 11 hours, so never idle for 12.
	for se.clock.Now().Add(11 * time.Hour).Before(s.ExpiresAt) {
		se.clock.Advance(11 * time.Hour)
		if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
			t.Fatalf("at %s: %v", se.clock.Now().Sub(t0), err)
		}
	}
	se.clock.Advance(s.ExpiresAt.Sub(se.clock.Now()) - time.Second)
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Fatalf("one second before 7 days: %v", err)
	}
	se.clock.Advance(time.Second)
	_, err := se.sessions.Lookup(context.Background(), token)
	mustBeUnauthenticated(t, err)
}

func TestNFK11_TouchWritesAtMostOncePerInterval(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)

	version := func() int32 {
		var v int32
		if err := se.pool.QueryRow(context.Background(), `SELECT version FROM sessions WHERE id = $1`, s.ID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	se.clock.Advance(30 * time.Second)
	for range 5 {
		if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	if v := version(); v != 1 {
		t.Errorf("version = %d after lookups within one minute, want 1: every request wrote", v)
	}
	se.clock.Advance(30 * time.Second)
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if v := version(); v != 2 {
		t.Errorf("version = %d after a minute, want 2", v)
	}
}

func TestNFK11_TouchNeverMovesTheAbsoluteLimit(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)

	for range 3 {
		se.clock.Advance(2 * time.Hour)
		if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
			t.Fatal(err)
		}
	}
	var expires time.Time
	if err := se.pool.QueryRow(context.Background(), `SELECT expires_at FROM sessions WHERE id = $1`, s.ID).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	if !expires.Equal(s.ExpiresAt) {
		t.Errorf("expires_at moved from %s to %s", s.ExpiresAt, expires)
	}
	// And the app role cannot move it, whatever the code does.
	_, err := se.pool.Exec(context.Background(), `UPDATE sessions SET expires_at = expires_at + interval '30 days' WHERE id = $1`, s.ID)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("app role UPDATE of expires_at: err = %v, want permission denied", err)
	}
	_, err = se.pool.Exec(context.Background(), `UPDATE sessions SET user_id = $2, roles = '{admin}' WHERE id = $1`, s.ID, u.ID)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("app role UPDATE of user_id/roles: err = %v, want permission denied", err)
	}
}

func TestNFK11_LookupRefusesMalformedAndUnknownTokens(t *testing.T) {
	se := newSessionEnv(t)
	for _, bad := range []string{"", "junk", strings.Repeat("a", 500)} {
		_, err := se.sessions.Lookup(context.Background(), bad)
		mustBeUnauthenticated(t, err)
	}
	unknown, _, _ := auth.NewToken()
	_, err := se.sessions.Lookup(context.Background(), unknown)
	mustBeUnauthenticated(t, err)

	if n := counter(t, se.reg, "linochi_auth_session_checks_total", map[string]string{"result": "malformed"}); n != 3 {
		t.Errorf("malformed checks counted = %v, want 3", n)
	}
	if n := counter(t, se.reg, "linochi_auth_session_checks_total", map[string]string{"result": "unknown"}); n != 1 {
		t.Errorf("unknown checks counted = %v, want 1", n)
	}
}

func TestNFK11_InvalidateAllEndsEverySessionOfOneUser(t *testing.T) {
	se := newSessionEnv(t)
	anna, bo := se.newUser(t, "anna"), se.newUser(t, "bo")
	a1, _ := se.login(t, anna.ID)
	a2, _ := se.login(t, anna.ID)
	bToken, _ := se.login(t, bo.ID)

	// Nobody in the context: an unattributed change is not saved (INS-05).
	if _, err := se.sessions.RevokeAllForUser(context.Background(), anna.ID); !errors.Is(err, audit.ErrNoActor) {
		t.Fatalf("without an actor: err = %v, want audit.ErrNoActor", err)
	}
	if n := se.countSessions(t, anna.ID); n != 2 {
		t.Fatalf("a refused revoke removed sessions: %d left, want 2", n)
	}

	ctx := audit.WithActor(context.Background(), audit.SystemActor("cli-revoke-user"))
	n, err := se.sessions.RevokeAllForUser(ctx, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("revoked %d, want 2", n)
	}
	for _, token := range []string{a1, a2} {
		_, err := se.sessions.Lookup(context.Background(), token)
		mustBeUnauthenticated(t, err)
	}
	if _, err := se.sessions.Lookup(context.Background(), bToken); err != nil {
		t.Errorf("another user's session was ended: %v", err)
	}

	rows := se.auditFor(t, "user", anna.ID)
	var found *dbgen.AuditLog
	for i := range rows {
		if rows[i].Action == "sessions_revoked" {
			found = &rows[i]
		}
	}
	if found == nil {
		t.Fatal("no sessions_revoked entry on the user")
	}
	if found.ActorType != "system" || found.ActorLabel != "cli-revoke-user" {
		t.Errorf("actor = %s/%s, want system/cli-revoke-user", found.ActorType, found.ActorLabel)
	}
	if got := keysOf(t, found.After)["count"]; got != float64(2) {
		t.Errorf("audit count = %v, want 2", got)
	}
}

func TestNFK11_InvalidateAllWithNoSessionsIsFineAndWritesNothing(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	before := len(se.auditFor(t, "user", u.ID))
	ctx := audit.WithActor(context.Background(), audit.SystemActor("cli-revoke-user"))
	n, err := se.sessions.RevokeAllForUser(ctx, u.ID)
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v, want 0 and nil", n, err)
	}
	if after := len(se.auditFor(t, "user", u.ID)); after != before {
		t.Errorf("an empty revoke wrote %d audit entries", after-before)
	}
}

func TestNFK11_ArchiveEndsAllSessionsInTheSameTransaction(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	t1, _ := se.login(t, u.ID)
	t2, _ := se.login(t, u.ID)

	if _, err := se.users.Archive(adminCtx(), u.ID, u.Version); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{t1, t2} {
		_, err := se.sessions.Lookup(context.Background(), token)
		mustBeUnauthenticated(t, err)
	}
	if n := se.countSessions(t, u.ID); n != 0 {
		t.Errorf("%d sessions left after the user was archived", n)
	}
	var revoked bool
	for _, r := range se.auditFor(t, "user", u.ID) {
		revoked = revoked || r.Action == "sessions_revoked"
	}
	if !revoked {
		t.Error("archiving ended sessions without an audit entry saying so")
	}
}

func TestNFK11_ArchivedUserCannotGetASession(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	if _, err := se.users.Archive(adminCtx(), u.ID, u.Version); err != nil {
		t.Fatal(err)
	}
	_, _, err := se.sessions.Create(context.Background(), u.ID, []string{"admin"})
	if !errors.Is(err, auth.ErrUserArchived) {
		t.Fatalf("Create for an archived user: err = %v, want auth.ErrUserArchived", err)
	}
	if n := se.countSessions(t, u.ID); n != 0 {
		t.Errorf("%d sessions exist for an archived user", n)
	}
	_, _, err = se.sessions.Create(context.Background(), uuid.Must(uuid.NewV7()), []string{"admin"})
	if !errors.Is(err, db.ErrNotFound) {
		t.Errorf("Create for an unknown user: err = %v, want db.ErrNotFound", err)
	}
}

func TestNFK11_LookupRefusesASessionWhoseUserWasArchivedBehindItsBack(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)
	// Not through UserStore.Archive: some other writer set archived_at.
	if _, err := se.pool.Exec(context.Background(), `UPDATE users SET archived_at = now() WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	_, err := se.sessions.Lookup(context.Background(), token)
	mustBeUnauthenticated(t, err)
	if n := counter(t, se.reg, "linochi_auth_session_checks_total", map[string]string{"result": "user_archived"}); n != 1 {
		t.Errorf("user_archived checks counted = %v, want 1", n)
	}
}

func TestNFK11_ArchiveAndLoginAtTheSameTimeLeaveNoSession(t *testing.T) {
	se := newSessionEnv(t)
	for i := range 25 {
		u := se.newUser(t, "racer-"+uuid.NewString())
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, err := se.sessions.Create(context.Background(), u.ID, []string{"admin"})
			if err != nil && !errors.Is(err, auth.ErrUserArchived) {
				t.Errorf("round %d: Create: %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := se.users.Archive(adminCtx(), u.ID, u.Version); err != nil {
				t.Errorf("round %d: Archive: %v", i, err)
			}
		}()
		wg.Wait()
		if n := se.countSessions(t, u.ID); n != 0 {
			t.Fatalf("round %d: %d sessions survive for an archived user", i, n)
		}
	}
}

func TestNFK09_CleanupDeletesOnlyEndedSessions(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	idleToken, _ := se.login(t, u.ID)
	oldToken, old := se.login(t, u.ID)

	// Keep the second session busy for the whole week; leave the first alone.
	for se.clock.Now().Add(11 * time.Hour).Before(old.ExpiresAt) {
		se.clock.Advance(11 * time.Hour)
		if _, err := se.sessions.Lookup(context.Background(), oldToken); err != nil {
			t.Fatal(err)
		}
	}
	se.clock.Advance(old.ExpiresAt.Sub(se.clock.Now()) - time.Second)
	freshToken, _ := se.login(t, u.ID) // created one second before the old one ends

	n, err := se.sessions.Cleanup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleanup at expires_at-1s deleted %d, want 1 (the idle one)", n)
	}
	se.clock.Advance(time.Second) // exactly the old session's limit
	n, err = se.sessions.Cleanup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("cleanup at expires_at deleted %d, want 1 (the absolute one)", n)
	}
	if _, err := se.sessions.Lookup(context.Background(), freshToken); err != nil {
		t.Errorf("cleanup removed a live session: %v", err)
	}
	for _, token := range []string{idleToken, oldToken} {
		_, err := se.sessions.Lookup(context.Background(), token)
		mustBeUnauthenticated(t, err)
	}
	if got := counter(t, se.reg, "linochi_auth_sessions_deleted_total", map[string]string{"reason": "cleanup"}); got != 2 {
		t.Errorf("deleted counter = %v, want 2", got)
	}
}

func TestNFK09_CleanupIsIdempotentAcrossInstances(t *testing.T) {
	se := newSessionEnv(t)
	podB, _ := se.another(t)
	u := se.newUser(t, "anna")
	for range 10 {
		se.login(t, u.ID)
	}
	se.clock.Advance(13 * time.Hour)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	for _, s := range []*auth.SessionStore{se.sessions, podB, se.sessions, podB} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := s.Cleanup(context.Background())
			if err != nil {
				t.Errorf("cleanup: %v", err)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	if total != 10 {
		t.Errorf("four concurrent cleanups removed %d rows in total, want exactly 10", total)
	}
}

func TestNFK09_AppRoleCanDeleteSessionsButNothingElse(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	_, s := se.login(t, u.ID)
	ctx := context.Background()
	if _, err := se.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, s.ID); err != nil {
		t.Errorf("app role cannot delete a session: %v", err)
	}
	if _, err := se.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("the DELETE grant leaked to users: err = %v", err)
	}
	if _, err := se.pool.Exec(ctx, `TRUNCATE sessions`); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("app role TRUNCATE sessions: err = %v, want permission denied", err)
	}
}

func TestINS05_LoginAndLogoutAreAuditedAndNeverHoldTheTokenHash(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)
	if err := se.sessions.Logout(context.Background(), token); err != nil {
		t.Fatal(err)
	}

	rows := se.auditFor(t, "session", s.ID)
	if len(rows) != 2 || rows[0].Action != audit.Create || rows[1].Action != "logout" {
		var actions []string
		for _, r := range rows {
			actions = append(actions, r.Action)
		}
		t.Fatalf("audit actions = %v, want [create logout]", actions)
	}
	hash := sha256.Sum256([]byte(token))
	forbidden := []string{token, "token_hash", hex.EncodeToString(hash[:]),
		base64.StdEncoding.EncodeToString(hash[:]), base64.RawURLEncoding.EncodeToString(hash[:])}
	for _, r := range rows {
		if r.ActorType != "user" || r.ActorID == nil || *r.ActorID != u.ID {
			t.Errorf("%s: actor = %s/%v, want the user themself", r.Action, r.ActorType, r.ActorID)
		}
		text := string(r.Before) + string(r.After)
		for _, f := range forbidden {
			if strings.Contains(text, f) {
				t.Errorf("%s entry contains %q: it could never be redacted out of an append-only log", r.Action, f)
			}
		}
	}
	if got := keysOf(t, rows[0].After); got["user_id"] != u.ID.String() {
		t.Errorf("the login entry does not say whose session it is: %v", got)
	}
	if keysOf(t, rows[1].Before) == nil {
		t.Error("the logout entry has no before: a logout with nothing to compare is never written")
	}
	// The log line to Loki holds names only.
	if strings.Contains(se.sessionLogs.String(), token) {
		t.Error("the token is in the log output")
	}
}

func TestINS05_LogoutOfAnUnknownSessionIsHarmlessAndNotAudited(t *testing.T) {
	se := newSessionEnv(t)
	unknown, _, _ := auth.NewToken()
	if err := se.sessions.Logout(context.Background(), unknown); err != nil {
		t.Errorf("Logout of an unknown token: %v, want nil (the browser's cookie is cleared either way)", err)
	}
	if err := se.sessions.Logout(context.Background(), "junk"); err != nil {
		t.Errorf("Logout of a malformed token: %v", err)
	}
	var n int
	if err := se.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type = 'session'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d session audit entries for logouts that ended nothing", n)
	}
}

func TestNFK02_FailedLoginLeavesNoSession(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	// A cancelled context fails the transaction somewhere inside Create. Where
	// exactly is not the point: the session and its audit entry are saved
	// together or not at all, so neither may exist afterwards.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := se.sessions.Create(ctx, u.ID, []string{"admin"}); err == nil {
		t.Fatal("Create on a cancelled context succeeded")
	}
	if n := se.countSessions(t, u.ID); n != 0 {
		t.Errorf("%d sessions after a failed login", n)
	}
	if rows := se.auditFor(t, "user", u.ID); len(rows) != 1 || rows[0].Action != audit.Create {
		t.Errorf("a failed login left %d audit entries on the user besides its creation", len(rows)-1)
	}
}
