//go:build integration

package auth_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// attemptEnv is a database, a clock and a login attempt store on it. another()
// is a second "pod" on a pool of its own.
type attemptEnv struct {
	*env
	clock    *testClock
	attempts *auth.LoginAttemptStore
}

func newAttemptEnv(t *testing.T, opts ...auth.LoginAttemptOption) *attemptEnv {
	t.Helper()
	ae := &attemptEnv{env: newEnv(t), clock: &testClock{t: t0}}
	ae.attempts = ae.another(t, opts...)
	return ae
}

func (ae *attemptEnv) another(t *testing.T, opts ...auth.LoginAttemptOption) *auth.LoginAttemptStore {
	t.Helper()
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{URL: ae.pool.Config().ConnString(), MaxConns: 4, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	opts = append([]auth.LoginAttemptOption{auth.WithAttemptClock(ae.clock.Now)}, opts...)
	return auth.NewLoginAttemptStore(pool, opts...)
}

func (ae *attemptEnv) rows(t *testing.T) int {
	t.Helper()
	var n int
	if err := ae.pool.QueryRow(context.Background(), `SELECT count(*) FROM oidc_login_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNFK10_LoginAttemptGivesBackWhatWasStoredAndWorksOnce(t *testing.T) {
	ae := newAttemptEnv(t)
	ctx := context.Background()
	p, err := ae.attempts.Begin(ctx, "/products/42")
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"state": p.State, "binding": p.Binding, "nonce": p.Nonce, "verifier": p.Verifier} {
		if len(v) < 43 {
			t.Errorf("%s = %q: too short to be unguessable", name, v)
		}
	}
	if p.State == p.Binding || p.State == p.Nonce || p.Nonce == p.Verifier {
		t.Error("two of state, binding, nonce and verifier are the same value")
	}

	got, err := ae.attempts.Finish(ctx, p.State, p.Binding)
	if err != nil {
		t.Fatalf("first finish: %v", err)
	}
	if got.Nonce != p.Nonce || got.Verifier != p.Verifier || got.ReturnTo != "/products/42" {
		t.Errorf("finished = %+v, want the nonce, verifier and return_to that were stored", got)
	}
	// The second use of the same state, even with the right binding, is refused.
	if _, err := ae.attempts.Finish(ctx, p.State, p.Binding); !errors.Is(err, auth.ErrLoginAttemptUnknown) {
		t.Errorf("second finish: err = %v, want ErrLoginAttemptUnknown", err)
	}
	if n := ae.rows(t); n != 0 {
		t.Errorf("%d rows left after the attempt was used", n)
	}
}

// Two replicas, or two requests, getting the same callback: exactly one wins.
func TestNFK10_LoginAttemptStateWorksExactlyOnceUnderConcurrency(t *testing.T) {
	ae := newAttemptEnv(t)
	pods := []*auth.LoginAttemptStore{ae.attempts, ae.another(t), ae.another(t)}
	p, err := pods[0].Begin(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}

	const callers = 12
	var wg sync.WaitGroup
	results := make(chan error, callers)
	start := make(chan struct{})
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := pods[i%len(pods)].Finish(context.Background(), p.State, p.Binding)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, auth.ErrLoginAttemptUnknown):
			t.Errorf("a losing call failed with %v, want ErrLoginAttemptUnknown", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d of %d callers finished the same login, want exactly 1", wins, callers)
	}
}

func TestNFK10_LoginAttemptExpiresAfterTenMinutes(t *testing.T) {
	ae := newAttemptEnv(t)
	ctx := context.Background()

	fresh, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	ae.clock.Advance(auth.LoginAttemptTTL - time.Second)
	if _, err := ae.attempts.Finish(ctx, fresh.State, fresh.Binding); err != nil {
		t.Errorf("an attempt one second before its limit: %v", err)
	}

	late, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	ae.clock.Advance(auth.LoginAttemptTTL) // exactly at the limit: over
	if _, err := ae.attempts.Finish(ctx, late.State, late.Binding); !errors.Is(err, auth.ErrLoginAttemptExpired) {
		t.Errorf("an attempt at its limit: err = %v, want ErrLoginAttemptExpired", err)
	}
	// An expired attempt is consumed too: it cannot be tried again.
	if _, err := ae.attempts.Finish(ctx, late.State, late.Binding); !errors.Is(err, auth.ErrLoginAttemptUnknown) {
		t.Errorf("second try of an expired attempt: err = %v, want ErrLoginAttemptUnknown", err)
	}
}

// A callback from another browser than the one that started the login has the
// right state and the wrong cookie. It is refused, and the attempt is gone, so
// the real browser cannot be used to retry it either.
func TestNFK10_LoginAttemptRefusesAnotherBrowserAndIsConsumed(t *testing.T) {
	ae := newAttemptEnv(t)
	ctx := context.Background()
	p, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	other, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}

	for name, binding := range map[string]string{"another browser's cookie": other.Binding, "no cookie": "", "junk": "junk"} {
		q, err := ae.attempts.Begin(ctx, "/")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ae.attempts.Finish(ctx, q.State, binding); !errors.Is(err, auth.ErrLoginBindingMismatch) {
			t.Errorf("%s: err = %v, want ErrLoginBindingMismatch", name, err)
		}
		if _, err := ae.attempts.Finish(ctx, q.State, q.Binding); !errors.Is(err, auth.ErrLoginAttemptUnknown) {
			t.Errorf("%s: the attempt survived a refused callback: err = %v", name, err)
		}
	}
	// None of that touched the unrelated attempt.
	if _, err := ae.attempts.Finish(ctx, p.State, p.Binding); err != nil {
		t.Errorf("an unrelated attempt was affected: %v", err)
	}
}

func TestNFK10_LoginAttemptUnknownOrMalformedStateIsRefused(t *testing.T) {
	ae := newAttemptEnv(t)
	other, err := ae.attempts.Begin(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	for name, state := range map[string]string{"empty": "", "junk": "not-a-state", "well-formed but unknown": other.Binding, "huge": strings.Repeat("a", 1<<16)} {
		if _, err := ae.attempts.Finish(context.Background(), state, other.Binding); !errors.Is(err, auth.ErrLoginAttemptUnknown) {
			t.Errorf("%s: err = %v, want ErrLoginAttemptUnknown", name, err)
		}
	}
	if n := ae.rows(t); n != 1 {
		t.Errorf("%d rows, want 1: refusing junk must not delete anything", n)
	}
}

// Only hashes are stored of what the browser holds: state and binding.
func TestNFK10_LoginAttemptStoresOnlyHashesOfStateAndBinding(t *testing.T) {
	ae := newAttemptEnv(t)
	p, err := ae.attempts.Begin(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	var stateHash, bindingHash []byte
	var asText string
	err = ae.pool.QueryRow(context.Background(),
		`SELECT state_hash, binding_hash, t::text FROM oidc_login_attempts t`).Scan(&stateHash, &bindingHash, &asText)
	if err != nil {
		t.Fatal(err)
	}
	if string(stateHash) == p.State || string(bindingHash) == p.Binding || len(stateHash) != 32 || len(bindingHash) != 32 {
		t.Error("state or binding is stored as it is, not as a hash")
	}
	if string(stateHash) != string(auth.HashToken(p.State)) || string(bindingHash) != string(auth.HashToken(p.Binding)) {
		t.Error("the stored values are not the SHA-256 of state and binding")
	}
	if strings.Contains(asText, p.State) || strings.Contains(asText, p.Binding) {
		t.Error("the state or the binding appears in the row")
	}
}

func TestNFK10_LoginAttemptsAreCappedAndExpiredOnesDoNotCount(t *testing.T) {
	ae := newAttemptEnv(t, auth.WithMaxOpenAttempts(3))
	ctx := context.Background()
	for range 3 {
		if _, err := ae.attempts.Begin(ctx, "/"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ae.attempts.Begin(ctx, "/"); !errors.Is(err, auth.ErrTooManyLoginAttempts) {
		t.Fatalf("the fourth open attempt: err = %v, want ErrTooManyLoginAttempts", err)
	}
	// Ten minutes on, the three have expired and no longer take up room, even
	// though the cleanup has not run.
	ae.clock.Advance(auth.LoginAttemptTTL)
	if _, err := ae.attempts.Begin(ctx, "/"); err != nil {
		t.Errorf("after the others expired: %v", err)
	}
}

func TestNFK10_LoginAttemptCleanupDeletesExpiredOnesOnly(t *testing.T) {
	ae := newAttemptEnv(t)
	ctx := context.Background()
	old, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	ae.clock.Advance(auth.LoginAttemptTTL)
	live, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = old

	n, err := ae.attempts.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("cleanup = %d, %v, want 1 deleted", n, err)
	}
	if n, err := ae.attempts.Cleanup(ctx); err != nil || n != 0 {
		t.Errorf("second cleanup = %d, %v: it must be idempotent", n, err)
	}
	if _, err := ae.attempts.Finish(ctx, live.State, live.Binding); err != nil {
		t.Errorf("the cleanup deleted a live attempt: %v", err)
	}
}

// The grants are part of the design: DELETE here is on purpose, UPDATE and
// TRUNCATE are not given, and the grant did not leak to another table.
func TestNFK10_AppRoleCanDeleteLoginAttemptsButNotUpdateOrTruncate(t *testing.T) {
	ae := newAttemptEnv(t)
	ctx := context.Background()
	p, err := ae.attempts.Begin(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	for name, stmt := range map[string]string{
		"UPDATE":   `UPDATE oidc_login_attempts SET return_to = '/x'`,
		"TRUNCATE": `TRUNCATE oidc_login_attempts`,
	} {
		if _, err := ae.pool.Exec(ctx, stmt); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s as the app role: err = %v, want permission denied", name, err)
		}
	}
	if _, err := ae.attempts.Finish(ctx, p.State, p.Binding); err != nil {
		t.Errorf("DELETE as the app role: %v", err)
	}
	if _, err := ae.pool.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Error("the DELETE grant leaked to audit_log")
	}
}
