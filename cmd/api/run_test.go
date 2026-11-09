package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

func listen(t *testing.T) net.Listener {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// A request that is in flight when SIGTERM arrives must still get its answer,
// otherwise every rolling update drops requests (NFK-02).
func TestNFK02_ServerShutsDownGracefullyOnCancel(t *testing.T) {
	ln := listen(t)

	started := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "done")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serve(ctx, discardLogger(), ln, handler, 5*time.Second) }()

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- result{body: string(b), err: err}
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never started")
	}

	cancel() // what SIGTERM does

	select {
	case err := <-served:
		t.Fatalf("serve returned before the in-flight request finished: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)

	select {
	case r := <-got:
		if r.err != nil || r.body != "done" {
			t.Fatalf("in-flight request: body=%q err=%v", r.body, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no response for the in-flight request")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after shutdown")
	}
}

func TestNFK02_ServerShutdownIsBoundedByTimeout(t *testing.T) {
	ln := listen(t)
	started := make(chan struct{})
	stuck := make(chan struct{})
	defer close(stuck)
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-stuck
	})

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- serve(ctx, discardLogger(), ln, handler, 100*time.Millisecond) }()

	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	cancel()

	select {
	case err := <-served:
		if err == nil {
			t.Fatal("expected a shutdown timeout error, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve ignored the shutdown timeout")
	}
}

func TestINS07_VersionSubcommandPrintsLdflagsValues(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"api", "version"}, func(string) string { return "" }, &out, io.Discard)
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if got, want := strings.TrimSpace(out.String()), "dev · unknown"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNFK02_RunFailsWithConfigErrorNamingTheField(t *testing.T) {
	var errOut bytes.Buffer
	code := run(context.Background(), []string{"api"}, func(string) string { return "" }, io.Discard, &errOut)
	if code == 0 {
		t.Fatal("expected a non-zero exit code without DATABASE_URL")
	}
	if !strings.Contains(errOut.String(), "DATABASE_URL") {
		t.Errorf("stderr should name the field: %q", errOut.String())
	}
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func testHandlers(t *testing.T) (public, internal http.Handler, health *httpx.Health) {
	t.Helper()
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	health = &httpx.Health{}
	metrics := obs.NewMetrics("v0.1.0", "abc1234")
	// The stores have no pool: these tests never reach a route that uses one.
	rec, err := audit.NewRecorder(discardLogger(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionStore(nil, rec, metrics)
	if err != nil {
		t.Fatal(err)
	}
	authHandler, err := auth.NewHandler(auth.HandlerConfig{
		OIDC:     auth.NewOIDCClient(auth.OIDCConfig{Issuer: "https://auth.invalid", ClientID: "x", ClientSecret: "x", RedirectURL: "https://erp.invalid/auth/callback"}),
		Attempts: auth.NewLoginAttemptStore(nil), Users: auth.NewUserStore(nil, rec), Sessions: sessions,
		Logger: discardLogger(), Metrics: metrics,
	})
	if err != nil {
		t.Fatal(err)
	}
	public, internal = newHandlers(discardLogger(), metrics, health, tp, authHandler)
	return public, internal, health
}

// The auth routes of the OpenAPI document are on the public port and only there.
func TestNFK10_AuthRoutesAreOnThePublicPortOnly(t *testing.T) {
	public, internal, _ := testHandlers(t)
	pub, int_ := httptest.NewServer(public), httptest.NewServer(internal)
	defer pub.Close()
	defer int_.Close()

	if code, body := get(t, pub.URL+"/api/v1/me"); code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
		t.Errorf("public GET /api/v1/me = %d %s, want 401 as a problem", code, body)
	}
	for _, path := range []string{"/api/v1/me", "/auth/login", "/auth/callback"} {
		if code, _ := get(t, int_.URL+path); code != http.StatusNotFound {
			t.Errorf("internal %s = %d, want 404: the internal port has no login", path, code)
		}
	}
}

func TestNFK02_MetricsAndProbesOnlyOnInternalPort(t *testing.T) {
	public, internal, _ := testHandlers(t)
	pub, int_ := httptest.NewServer(public), httptest.NewServer(internal)
	defer pub.Close()
	defer int_.Close()

	for _, path := range []string{"/metrics", "/healthz", "/readyz"} {
		if code, _ := get(t, pub.URL+path); code != http.StatusNotFound {
			t.Errorf("public %s = %d, want 404: it must not be reachable through the ingress", path, code)
		}
	}
	if code, body := get(t, int_.URL+"/metrics"); code != 200 || !strings.Contains(body, "linochi_build_info") {
		t.Errorf("internal /metrics = %d, build_info present: %v", code, strings.Contains(body, "linochi_build_info"))
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		if code, _ := get(t, int_.URL+path); code != 200 {
			t.Errorf("internal %s = %d", path, code)
		}
	}
}

// On SIGTERM /readyz must turn 503 while the public port still answers, so that
// Traefik stops sending traffic before the listeners close (NFK-02).
func TestNFK02_ShutdownDrainsBeforeClosing(t *testing.T) {
	public, internal, health := testHandlers(t)
	pubLn, intLn := listen(t), listen(t)
	pubURL, intURL := "http://"+pubLn.Addr().String(), "http://"+intLn.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runServers(ctx, discardLogger(), health, 400*time.Millisecond, 5*time.Second,
			server{"public", pubLn, public}, server{"internal", intLn, internal})
	}()

	if code, _ := get(t, intURL+"/readyz"); code != 200 {
		t.Fatalf("/readyz before shutdown = %d", code)
	}
	cancel() // SIGTERM
	time.Sleep(100 * time.Millisecond)

	if code, _ := get(t, intURL+"/readyz"); code != http.StatusServiceUnavailable {
		t.Errorf("/readyz during the drain delay = %d, want 503", code)
	}
	if code, _ := get(t, pubURL+"/anything"); code != http.StatusNotFound {
		t.Errorf("public port during the drain delay = %d, want it still serving (404 from the router)", code)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServers: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runServers did not return")
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, intURL+"/readyz", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Error("the internal port still accepts connections after shutdown")
	}
}

func TestNFK02_OneServerFailingStopsTheOther(t *testing.T) {
	public, internal, health := testHandlers(t)
	pubLn, intLn := listen(t), listen(t)
	_ = pubLn.Close() // Serve on a closed listener fails at once

	done := make(chan error, 1)
	go func() {
		done <- runServers(context.Background(), discardLogger(), health, time.Hour, 2*time.Second,
			server{"public", pubLn, public}, server{"internal", intLn, internal})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected the listener failure to be returned")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a failing server did not stop the other one")
	}
}
