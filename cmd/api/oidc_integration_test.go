//go:build integration

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/testdb"
)

// wire is an HTTP client that does not follow redirects and keeps cookies by
// hand: the session cookie is Secure and these tests speak plain http on the
// loopback, which a cookie jar rightly refuses to send it over.
type wire struct {
	t       *testing.T
	base    string
	cookies map[string]string
}

// reply is what a test looks at of an answer; the body has been read and closed.
type reply struct {
	StatusCode int
	Header     http.Header
}

func (w *wire) get(target string) (reply, string) {
	w.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, w.base+target, nil)
	if err != nil {
		w.t.Fatal(err)
	}
	for k, v := range w.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		w.t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	for _, ck := range resp.Cookies() {
		if ck.MaxAge < 0 {
			delete(w.cookies, ck.Name)
		} else {
			w.cookies[ck.Name] = ck.Value
		}
	}
	return reply{StatusCode: resp.StatusCode, Header: resp.Header}, string(body)
}

func TestNFK10_TheRunningAppLogsInThroughTheProviderOverHTTP(t *testing.T) {
	owner := pgServer.NewMigratedDatabase(t)
	appAddr := freeAddr(t)
	redirect := "http://" + appAddr + "/auth/callback"
	idp := oidctest.Start(t, oidctest.Config{ClientID: "linochi-erp", ClientSecret: "a-client-secret", RedirectURL: redirect})

	// The provider is on loopback, so http is allowed; nothing else is.
	app, stop := startApp(t, testdb.AppURL(owner), map[string]string{
		"HTTP_ADDR": appAddr, "OIDC_ISSUER": idp.Issuer(), "OIDC_REDIRECT_URL": redirect,
	})
	w := &wire{t: t, base: "http://" + app.public, cookies: map[string]string{}}

	// Without a session there is nothing to see.
	if resp, _ := w.get("/api/v1/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without a session = %d, want 401", resp.StatusCode)
	}

	resp, _ := w.get("/auth/login?return_to=%2Forders")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), idp.Issuer()+"/authorize?") {
		t.Fatalf("login = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	cb, err := url.Parse(idp.Authorize(t, resp.Header.Get("Location")))
	if err != nil {
		t.Fatal(err)
	}
	resp, _ = w.get(cb.RequestURI())
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/orders" {
		t.Fatalf("callback = %d to %q, want 303 to /orders", resp.StatusCode, resp.Header.Get("Location"))
	}
	if w.cookies[auth.SessionCookieName] == "" {
		t.Fatal("no session cookie after the callback")
	}

	resp, body := w.get("/api/v1/me")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "anna.svensson@example.se") {
		t.Fatalf("me with a session = %d %s", resp.StatusCode, body)
	}

	// The login shows up where it is watched: the metric on the internal port.
	internal := &wire{t: t, base: "http://" + app.internal, cookies: map[string]string{}}
	_, metrics := internal.get("/metrics")
	if !strings.Contains(metrics, `linochi_auth_logins_total{result="ok"} 1`) {
		t.Errorf("/metrics has no ok login:\n%s", grepLines(metrics, "linochi_auth_logins_total"))
	}
	if exit := stop(); exit != 0 {
		t.Errorf("exit code %d after shutdown, want 0", exit)
	}
}

// The login is the only thing that needs Authelia: the app starts and stays
// ready while it is down, and a login then ends at the error page.
func TestNFK10_TheAppStartsAndStaysReadyWhileTheProviderIsDown(t *testing.T) {
	owner := pgServer.NewMigratedDatabase(t)
	app, stop := startApp(t, testdb.AppURL(owner), nil) // OIDC_ISSUER is https://auth.invalid: nobody answers

	deadline := time.Now().Add(10 * time.Second)
	var code int
	for time.Now().Before(deadline) {
		if code, _ = fetch(t, "http://"+app.internal+"/readyz"); code == 200 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if code != 200 {
		t.Fatalf("/readyz = %d with the provider down, want 200: a login outage must not take the app out of rotation", code)
	}
	w := &wire{t: t, base: "http://" + app.public, cookies: map[string]string{}}
	resp, _ := w.get("/auth/login")
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login-error?code=failed" {
		t.Errorf("login with the provider down = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if exit := stop(); exit != 0 {
		t.Errorf("exit code %d, want 0", exit)
	}
}

// lines is a log a process writes to and a test reads from, at the same time.
type lines struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lines) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.Write(p) }
func (l *lines) String() string              { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

// buildAPI builds the binary the way the image will: static, from this package.
func buildAPI(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "api")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// NFK-12: the app trusts the internal CA for *.lab.internal. In the cluster the
// CA is mounted from a ConfigMap and SSL_CERT_DIR points at it, as for Gitea and
// Grafana: Go adds those directories to the system roots. The process is a real
// one because Go reads the environment once, when the first certificate is
// checked, which a test inside another process cannot redo.
func TestNFK12_TheProcessTrustsTheInternalCAThroughSSLCertDir(t *testing.T) {
	owner := pgServer.NewMigratedDatabase(t)
	idp := oidctest.StartTLS(t, oidctest.Config{ClientID: "linochi-erp", ClientSecret: "a-client-secret", RedirectURL: "https://erp.invalid/auth/callback"})
	bin := buildAPI(t)

	caDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(caDir, "internal-ca.crt"), idp.CAPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	loginTarget := func(extraEnv ...string) (string, string) {
		t.Helper()
		pub, internal := freeAddr(t), freeAddr(t)
		logs := &lines{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cmd := exec.CommandContext(ctx, bin)
		cmd.Env = append([]string{
			"PATH=" + os.Getenv("PATH"), "DATABASE_URL=" + testdb.AppURL(owner), "HTTP_ADDR=" + pub, "INTERNAL_ADDR=" + internal,
			"SHUTDOWN_DELAY=0s", "LOG_LEVEL=debug",
			"OIDC_ISSUER=" + idp.Issuer(), "OIDC_CLIENT_ID=linochi-erp", "OIDC_CLIENT_SECRET=a-client-secret",
			"OIDC_REDIRECT_URL=https://erp.invalid/auth/callback",
		}, extraEnv...)
		cmd.Stderr, cmd.Stdout = logs, logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer cancel()

		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if code, _ := fetch(t, "http://"+internal+"/healthz"); code == 200 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		w := &wire{t: t, base: "http://" + pub, cookies: map[string]string{}}
		resp, _ := w.get("/auth/login")
		// The process is stopped before its log is read: what it wrote is copied to
		// logs by a goroutine of exec, which is done once Wait returns.
		cancel()
		_ = cmd.Wait()
		return resp.Header.Get("Location"), logs.String()
	}

	// With the CA the login is sent on to the provider.
	loc, logs := loginTarget("SSL_CERT_DIR=" + caDir)
	if !strings.HasPrefix(loc, idp.Issuer()+"/authorize?") {
		t.Fatalf("with the CA: login goes to %q; logs:\n%s", loc, logs)
	}
	// Without it the provider is not trusted: the login fails, and the log says
	// that it was the certificate.
	loc, logs = loginTarget()
	if loc != "/login-error?code=failed" {
		t.Fatalf("without the CA: login goes to %q, want the error page", loc)
	}
	if !strings.Contains(logs, "certificate") {
		t.Errorf("the log does not say it was the certificate:\n%s", logs)
	}
}

// The test provider signs in with a fixed key and approves everyone. It must not
// be possible to ship it by accident.
func TestNFK10_TheTestProviderIsNotInTheAPIBinary(t *testing.T) {
	out, err := exec.CommandContext(context.Background(), "go", "list", "-deps", "-f", "{{.ImportPath}}", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "oidctest") || strings.HasSuffix(pkg, "/cmd/devoidc") || pkg == "net/http/httptest" {
			t.Errorf("cmd/api depends on %s", pkg)
		}
	}
}

func grepLines(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
