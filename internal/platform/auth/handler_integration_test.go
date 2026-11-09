//go:build integration

package auth_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// handlerEnv is a database, a provider and the login routes on top of them. A
// pod is one replica of the backend: its own pool, stores and OIDC client, the
// same database and provider, so that the callback can be sent to another one.
type handlerEnv struct {
	*sessionEnv
	idp *oidctest.Server
}

type pod struct {
	mux  http.Handler
	reg  *prometheus.Registry
	logs *syncBuffer
}

func newHandlerEnv(t *testing.T) (*handlerEnv, *pod) {
	t.Helper()
	he := &handlerEnv{sessionEnv: newSessionEnv(t), idp: oidctest.Start(t, providerCfg)}
	return he, he.newPod(t)
}

func (he *handlerEnv) newPod(t *testing.T, attemptOpts ...auth.LoginAttemptOption) *pod {
	t.Helper()
	metrics := obs.NewMetrics("test", "abc1234")
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	pool, err := db.Open(context.Background(), db.Options{URL: he.pool.Config().ConnString(), MaxConns: 4, Metrics: metrics, Tracer: tp})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	logs := &syncBuffer{}
	logger := obs.NewLogger(logs, slog.LevelDebug)
	rec, err := audit.NewRecorder(logger, metrics)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionStore(pool, rec, metrics, auth.WithClock(he.clock.Now), auth.WithLogger(logger), auth.WithTracerProvider(tp))
	if err != nil {
		t.Fatal(err)
	}
	attemptOpts = append([]auth.LoginAttemptOption{auth.WithAttemptClock(he.clock.Now), auth.WithAttemptLogger(logger)}, attemptOpts...)
	h, err := auth.NewHandler(auth.HandlerConfig{
		OIDC: auth.NewOIDCClient(auth.OIDCConfig{
			Issuer: he.idp.Issuer(), ClientID: providerCfg.ClientID, ClientSecret: providerCfg.ClientSecret, RedirectURL: callbackURL,
		}, auth.WithOIDCHTTPClient(he.idp.Client()), auth.WithOIDCClock(he.clock.Now), auth.WithOIDCLogger(logger)),
		Attempts: auth.NewLoginAttemptStore(pool, attemptOpts...),
		Users:    auth.NewUserStore(pool, rec), Sessions: sessions,
		AdminGroups: []string{"admins", "linochi_erp_users"},
		Logger:      logger, Metrics: metrics, Now: he.clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Mount(mux)
	return &pod{mux: mux, reg: metrics.Registry, logs: logs}
}

// browser is a cookie jar in front of a pod, as far as the login needs one.
type browser struct {
	t       *testing.T
	pod     *pod
	cookies map[string]*http.Cookie
}

func (he *handlerEnv) browser(t *testing.T, p *pod) *browser {
	return &browser{t: t, pod: p, cookies: map[string]*http.Cookie{}}
}

func (b *browser) do(method, target string, headers map[string]string) *httptest.ResponseRecorder {
	b.t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), method, "https://erp.lab.internal"+target, nil)
	for _, c := range b.cookies {
		r.AddCookie(c)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	b.pod.mux.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c
		}
	}
	return w
}

func (b *browser) get(target string) *httptest.ResponseRecorder { return b.do("GET", target, nil) }

// start begins a login and returns where the browser is sent.
func (b *browser) start(returnTo string) string {
	b.t.Helper()
	target := "/auth/login"
	if returnTo != "" {
		target += "?return_to=" + url.QueryEscape(returnTo)
	}
	w := b.get(target)
	if w.Code != http.StatusSeeOther {
		b.t.Fatalf("GET /auth/login = %d, want 303; body: %s", w.Code, w.Body.String())
	}
	return w.Header().Get("Location")
}

// callbackTarget is what Authelia sends the browser back to, as a path and query.
func (he *handlerEnv) callbackTarget(t *testing.T, authorizeURL string) string {
	t.Helper()
	u, err := url.Parse(he.idp.Authorize(t, authorizeURL))
	if err != nil {
		t.Fatal(err)
	}
	return u.RequestURI()
}

// finish does the whole round trip after start: the provider approves and the
// browser comes back to the callback.
func (he *handlerEnv) finish(t *testing.T, b *browser, authorizeURL string) *httptest.ResponseRecorder {
	t.Helper()
	return b.get(he.callbackTarget(t, authorizeURL))
}

func (he *handlerEnv) login(t *testing.T, b *browser, returnTo string) *httptest.ResponseRecorder {
	t.Helper()
	return he.finish(t, b, b.start(returnTo))
}

func wantRedirect(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != want {
		t.Fatalf("answered %d to %q, want 303 to %q", w.Code, w.Header().Get("Location"), want)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q: a login step must not be cached", cc)
	}
}

func cookieNamed(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (he *handlerEnv) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := he.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func logins(t *testing.T, p *pod, result string) float64 {
	t.Helper()
	return counter(t, p.reg, "linochi_auth_logins_total", map[string]string{"result": result})
}

func TestNFK10_LoginFlowMakesASessionAndGoesBackToTheStartingPage(t *testing.T) {
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)
	he.idp.SetUser(oidctest.User{
		Subject: "sub-anna", Email: "anna.svensson@example.se", Name: "Anna Svensson", Groups: []string{"linochi_erp_users"},
	})

	authorizeURL := b.start("/products/42?tab=stock")
	if !strings.HasPrefix(authorizeURL, he.idp.Issuer()+"/authorize?") {
		t.Fatalf("login sends the browser to %q, want the provider's authorization endpoint", authorizeURL)
	}
	// The login is bound to this browser by a cookie that only the callback reads.
	bind := b.cookies[auth.LoginCookieName]
	if bind == nil || !bind.Secure || !bind.HttpOnly || bind.SameSite != http.SameSiteLaxMode || bind.Path != "/" || bind.Domain != "" ||
		bind.MaxAge != int(auth.LoginAttemptTTL/time.Second) || !strings.HasPrefix(bind.Name, "__Host-") {
		t.Fatalf("binding cookie = %+v", bind)
	}
	if he.count(t, "sessions") != 0 || he.count(t, "users") != 0 {
		t.Fatal("starting a login made a user or a session")
	}

	target := he.callbackTarget(t, authorizeURL)
	w := b.get(target)
	wantRedirect(t, w, "/products/42?tab=stock")

	sc := cookieNamed(w, auth.SessionCookieName)
	if sc == nil || !sc.Secure || !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode || sc.Path != "/" || sc.Domain != "" ||
		sc.MaxAge != int(auth.AbsoluteTimeout/time.Second) || len(sc.Value) != 43 {
		t.Fatalf("session cookie = %+v", sc)
	}
	if _, still := b.cookies[auth.LoginCookieName]; still {
		t.Error("the binding cookie was not cleared once the login was over")
	}

	// Who am I?
	me := b.get("/api/v1/me")
	if me.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/me = %d: %s", me.Code, me.Body.String())
	}
	var body struct {
		ID          uuid.UUID `json:"id"`
		Email       string    `json:"email"`
		DisplayName string    `json:"display_name"`
		Roles       []string  `json:"roles"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Email != "anna.svensson@example.se" || body.DisplayName != "Anna Svensson" || !slices.Equal(body.Roles, []string{"admin"}) {
		t.Errorf("me = %+v", body)
	}

	// What was stored: one user, one session with the role kept in it, and both
	// audited in the same transactions that made them.
	if he.count(t, "users") != 1 || he.count(t, "sessions") != 1 || he.count(t, "oidc_login_attempts") != 0 {
		t.Errorf("users=%d sessions=%d attempts=%d, want 1, 1 and 0", he.count(t, "users"), he.count(t, "sessions"), he.count(t, "oidc_login_attempts"))
	}
	var roles []string
	var sessID uuid.UUID
	if err := he.pool.QueryRow(context.Background(), `SELECT id, roles FROM sessions`).Scan(&sessID, &roles); err != nil || !slices.Equal(roles, []string{"admin"}) {
		t.Errorf("session roles = %v, err = %v", roles, err)
	}
	userRows := he.auditRows(t, body.ID)
	if len(userRows) != 1 || userRows[0].Action != audit.Create || userRows[0].ActorType != "system" || userRows[0].ActorLabel != "oidc-login" {
		t.Errorf("user audit = %+v", userRows)
	}
	sessRows, err := dbgen.New(he.pool).ListAuditEntriesForEntity(context.Background(), dbgen.ListAuditEntriesForEntityParams{EntityType: "session", EntityID: sessID})
	if err != nil || len(sessRows) != 1 || sessRows[0].Action != audit.Create || sessRows[0].ActorType != "user" {
		t.Errorf("session audit = %+v, err = %v", sessRows, err)
	}
	if got := logins(t, p, "ok"); got != 1 {
		t.Errorf("linochi_auth_logins_total{result=ok} = %v, want 1", got)
	}

	// Nothing the browser or the provider holds is in the log.
	state := mustQuery(t, target, "state")
	for name, secret := range map[string]string{
		"the state": state, "the code": mustQuery(t, target, "code"), "the binding cookie": bind.Value,
		"the session token": sc.Value, "the client secret": providerCfg.ClientSecret,
	} {
		if strings.Contains(p.logs.String(), secret) {
			t.Errorf("the log contains %s", name)
		}
	}
}

func mustQuery(t *testing.T, target, key string) string {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	v := u.Query().Get(key)
	if v == "" {
		t.Fatalf("no %s in %q", key, target)
	}
	return v
}

func TestNFK10_ReturnToThatIsNotAPathOfThisAppIsReplacedBySlash(t *testing.T) {
	for _, returnTo := range []string{"", "https://evil.example/phish", "//evil.example", "/\\evil.example", "javascript:alert(1)"} {
		he, p := newHandlerEnv(t)
		b := he.browser(t, p)
		wantRedirect(t, he.login(t, b, returnTo), "/")
	}
}

func TestNFK10_SecondLoginOfTheSameUserRefreshesTheProfileAndAddsASession(t *testing.T) {
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)
	he.idp.SetUser(oidctest.User{Subject: "sub-anna", Email: "anna@example.se", Name: "Anna", Groups: []string{"admins"}})
	wantRedirect(t, he.login(t, b, ""), "/")

	he.clock.Advance(time.Hour)
	he.idp.SetUser(oidctest.User{Subject: "sub-anna", Email: "anna.new@example.se", Name: "Anna Svensson", Groups: []string{"admins"}})
	wantRedirect(t, he.login(t, he.browser(t, p), ""), "/")

	if he.count(t, "users") != 1 || he.count(t, "sessions") != 2 {
		t.Fatalf("users=%d sessions=%d, want 1 and 2", he.count(t, "users"), he.count(t, "sessions"))
	}
	var email, name string
	var id uuid.UUID
	if err := he.pool.QueryRow(context.Background(), `SELECT id, email, display_name FROM users`).Scan(&id, &email, &name); err != nil {
		t.Fatal(err)
	}
	if email != "anna.new@example.se" || name != "Anna Svensson" {
		t.Errorf("profile = %q, %q: the second login did not refresh it", email, name)
	}
	var actions []string
	for _, r := range he.auditRows(t, id) {
		actions = append(actions, r.Action)
	}
	if !slices.Equal(actions, []string{audit.Create, "login"}) {
		t.Errorf("user audit actions = %v, want create then login", actions)
	}
}

// The callback can land on any replica (NFK-02): the attempt is in Postgres.
func TestNFK10_TheCallbackMayLandOnAnotherReplicaThanTheLoginStarted(t *testing.T) {
	he, podA := newHandlerEnv(t)
	podB := he.newPod(t)
	b := he.browser(t, podA)
	authorizeURL := b.start("/orders")

	b.pod = podB
	wantRedirect(t, he.finish(t, b, authorizeURL), "/orders")
	// And the session works on the one that did not make it.
	b.pod = podA
	if w := b.get("/api/v1/me"); w.Code != http.StatusOK {
		t.Errorf("GET /api/v1/me on the first replica = %d", w.Code)
	}
}

// The browser is told a fixed code and nothing else, whatever went wrong.
func TestNFK10_FailedLoginsAreToldToTheBrowserAsFixedCodesOnly(t *testing.T) {
	type tc struct {
		name   string
		setup  func(he *handlerEnv)
		code   string
		reason auth.Reason
	}
	cases := []tc{
		{"no group gives no role", func(he *handlerEnv) {
			he.idp.SetUser(oidctest.User{Subject: "s", Email: "a@example.se", Groups: []string{"family"}})
		}, "no-access", auth.ReasonNoRole},
		{"no groups at all", func(he *handlerEnv) {
			he.idp.SetUser(oidctest.User{Subject: "s", Email: "a@example.se"})
		}, "no-access", auth.ReasonNoRole},
		{"only one factor", func(he *handlerEnv) { he.idp.Configure(func(b *oidctest.Behavior) { b.AMR = []string{"pwd"} }) }, "no-access", auth.ReasonMFARequired},
		{"Authelia says no", func(he *handlerEnv) {
			he.idp.Configure(func(b *oidctest.Behavior) { b.DenyAuthorize = "access_denied" })
		}, "no-access", auth.ReasonProviderDenied},
		{"Authelia has an error", func(he *handlerEnv) {
			he.idp.Configure(func(b *oidctest.Behavior) { b.DenyAuthorize = "server_error" })
		}, "failed", auth.ReasonProviderError},
		{"a token for another login", func(he *handlerEnv) { he.idp.Configure(func(b *oidctest.Behavior) { b.Nonce = "another" }) }, "failed", auth.ReasonNonceMismatch},
		{"a token from another issuer", func(he *handlerEnv) {
			he.idp.Configure(func(b *oidctest.Behavior) { b.Issuer = "https://evil.example" })
		}, "failed", auth.ReasonTokenInvalid},
		{"the token endpoint is down", func(he *handlerEnv) { he.idp.Configure(func(b *oidctest.Behavior) { b.TokenStatus = 502 }) }, "failed", auth.ReasonExchangeFailed},
		{"userinfo is down", func(he *handlerEnv) { he.idp.Configure(func(b *oidctest.Behavior) { b.UserInfoStatus = 500 }) }, "failed", auth.ReasonUserInfoFailed},
		{"userinfo is about someone else", func(he *handlerEnv) { he.idp.Configure(func(b *oidctest.Behavior) { b.UserInfoSubject = "x" }) }, "failed", auth.ReasonSubjectMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			he, p := newHandlerEnv(t)
			he.idp.SetUser(oidctest.DefaultUser)
			c.setup(he)
			b := he.browser(t, p)
			w := he.login(t, b, "/products")

			wantRedirect(t, w, "/login-error?code="+c.code)
			// No detail in anything the browser gets.
			if body := w.Body.String(); strings.Contains(body, "nonce") || strings.Contains(body, "token") || strings.Contains(body, "issuer") {
				t.Errorf("the body says too much: %q", body)
			}
			if cookieNamed(w, auth.SessionCookieName) != nil {
				t.Error("a failed login set a session cookie")
			}
			if _, still := b.cookies[auth.LoginCookieName]; still {
				t.Error("the binding cookie outlived a failed login")
			}
			// Nothing was made: no user, no session, no audit entry, and the attempt is used up.
			for _, table := range []string{"users", "sessions", "audit_log", "oidc_login_attempts"} {
				if n := he.count(t, table); n != 0 {
					t.Errorf("%d rows in %s after a failed login", n, table)
				}
			}
			if got := logins(t, p, string(c.reason)); got != 1 {
				t.Errorf("linochi_auth_logins_total{result=%s} = %v, want 1", c.reason, got)
			}
			if !strings.Contains(p.logs.String(), string(c.reason)) {
				t.Errorf("the log does not say why (%s):\n%s", c.reason, p.logs.String())
			}
		})
	}
}

func TestNFK10_ArchivedUserGetsNoAccessAndNothingIsWritten(t *testing.T) {
	he, p := newHandlerEnv(t)
	wantRedirect(t, he.login(t, he.browser(t, p), ""), "/")
	var u dbgen.User
	if err := he.pool.QueryRow(context.Background(), `SELECT id, version FROM users`).Scan(&u.ID, &u.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := he.users.Archive(adminCtx(), u.ID, u.Version); err != nil {
		t.Fatal(err)
	}
	auditBefore := he.count(t, "audit_log")

	he.idp.SetUser(oidctest.User{Subject: oidctest.DefaultUser.Subject, Email: "changed@example.se", Name: "Changed", Groups: []string{"admins"}})
	wantRedirect(t, he.login(t, he.browser(t, p), ""), "/login-error?code=no-access")

	if he.count(t, "sessions") != 0 {
		t.Error("an archived user got a session")
	}
	if n := he.count(t, "audit_log"); n != auditBefore {
		t.Errorf("audit rows %d -> %d: a refused login must not be recorded", auditBefore, n)
	}
	var email string
	if err := he.pool.QueryRow(context.Background(), `SELECT email FROM users WHERE id = $1`, u.ID).Scan(&email); err != nil || email == "changed@example.se" {
		t.Errorf("email = %q, err = %v: the profile of an archived user was refreshed", email, err)
	}
	if got := logins(t, p, "user_archived"); got != 1 {
		t.Errorf("user_archived = %v, want 1", got)
	}
}

func TestNFK10_StateWorksOnceAndIsBoundToTheBrowserThatStartedTheLogin(t *testing.T) {
	t.Run("a second use of the callback", func(t *testing.T) {
		he, p := newHandlerEnv(t)
		b := he.browser(t, p)
		target := he.callbackTarget(t, b.start(""))
		wantRedirect(t, b.get(target), "/")

		again := he.browser(t, p)
		wantRedirect(t, again.get(target), "/login-error?code=expired")
		// Not even with the binding cookie of the first time.
		for _, c := range b.cookies {
			again.cookies[c.Name] = c
		}
		wantRedirect(t, again.get(target), "/login-error?code=expired")
		if he.count(t, "sessions") != 1 {
			t.Errorf("%d sessions, want only the first login's", he.count(t, "sessions"))
		}
	})
	t.Run("another browser than the one that started it", func(t *testing.T) {
		he, p := newHandlerEnv(t)
		victim, attacker := he.browser(t, p), he.browser(t, p)
		// The attacker starts a login and gets the victim to open its callback.
		target := he.callbackTarget(t, attacker.start(""))
		victim.start("") // the victim has a login of their own going, with a cookie of their own
		wantRedirect(t, victim.get(target), "/login-error?code=failed")
		if he.count(t, "sessions") != 0 || he.count(t, "users") != 0 {
			t.Error("a callback from the wrong browser logged somebody in")
		}
		if got := logins(t, p, "binding_mismatch"); got != 1 {
			t.Errorf("binding_mismatch = %v, want 1", got)
		}
		// The attempt is gone: the attacker cannot finish it either.
		wantRedirect(t, attacker.get(target), "/login-error?code=expired")
	})
	t.Run("a browser with no cookie at all", func(t *testing.T) {
		he, p := newHandlerEnv(t)
		target := he.callbackTarget(t, he.browser(t, p).start(""))
		wantRedirect(t, he.browser(t, p).get(target), "/login-error?code=failed")
	})
	t.Run("a login that took too long", func(t *testing.T) {
		he, p := newHandlerEnv(t)
		b := he.browser(t, p)
		authorizeURL := b.start("")
		he.clock.Advance(auth.LoginAttemptTTL)
		wantRedirect(t, he.finish(t, b, authorizeURL), "/login-error?code=expired")
		if got := logins(t, p, "attempt_expired"); got != 1 {
			t.Errorf("attempt_expired = %v, want 1", got)
		}
	})
	t.Run("junk and missing parameters", func(t *testing.T) {
		he, p := newHandlerEnv(t)
		b := he.browser(t, p)
		for target, want := range map[string]string{
			"/auth/callback":                   "failed",
			"/auth/callback?code=x":            "failed",
			"/auth/callback?state=junk&code=x": "expired",
			"/auth/callback?state=" + strings.Repeat("a", 5000) + "&code=x": "expired",
			"/auth/callback?error=access_denied":                            "failed",
		} {
			wantRedirect(t, b.get(target), "/login-error?code="+want)
		}
		// A real state with no code: the attempt is used up, and it is not a login.
		b2 := he.browser(t, p)
		authorizeURL := b2.start("")
		u, _ := url.Parse(he.idp.Authorize(t, authorizeURL))
		wantRedirect(t, b2.get("/auth/callback?state="+url.QueryEscape(u.Query().Get("state"))), "/login-error?code=failed")
		if he.count(t, "oidc_login_attempts") != 0 {
			t.Error("the attempt survived a callback")
		}
	})
}

func TestNFK10_ProviderDownGivesAFailedLoginAndRecoversWithoutARestart(t *testing.T) {
	he, p := newHandlerEnv(t)
	he.idp.Configure(func(b *oidctest.Behavior) { b.DiscoveryStatus = http.StatusServiceUnavailable })
	b := he.browser(t, p)

	w := b.get("/auth/login")
	wantRedirect(t, w, "/login-error?code=failed")
	if cookieNamed(w, auth.LoginCookieName) != nil || he.count(t, "oidc_login_attempts") != 0 {
		t.Error("a login that could not start left a cookie or a row behind")
	}
	if got := logins(t, p, "provider_unavailable"); got != 1 {
		t.Errorf("provider_unavailable = %v, want 1", got)
	}

	he.idp.Configure(func(b *oidctest.Behavior) { b.DiscoveryStatus = 0 })
	he.clock.Advance(auth.DiscoveryRetryDelay)
	wantRedirect(t, he.login(t, b, ""), "/")
}

func TestNFK10_TooManyLoginsInProgressIsAFailedLoginAndNotAFlood(t *testing.T) {
	he, _ := newHandlerEnv(t)
	p := he.newPod(t, auth.WithMaxOpenAttempts(2))
	for range 2 {
		he.browser(t, p).start("")
	}
	wantRedirect(t, he.browser(t, p).get("/auth/login"), "/login-error?code=failed")
	if got := logins(t, p, "too_many_attempts"); got != 1 {
		t.Errorf("too_many_attempts = %v, want 1", got)
	}
	if he.count(t, "oidc_login_attempts") != 2 {
		t.Errorf("%d attempts stored, want the cap, 2", he.count(t, "oidc_login_attempts"))
	}
}

func TestNFK10_MeNeedsASessionAndLogoutEndsIt(t *testing.T) {
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)
	if w := b.get("/api/v1/me"); w.Code != http.StatusUnauthorized {
		t.Fatalf("me without a session = %d, want 401", w.Code)
	}
	he.login(t, b, "")
	if w := b.get("/api/v1/me"); w.Code != http.StatusOK {
		t.Fatalf("me with a session = %d, want 200", w.Code)
	}
	token := b.cookies[auth.SessionCookieName].Value

	if w := b.do("POST", "/auth/logout", nil); w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", w.Code)
	}
	if w := b.get("/api/v1/me"); w.Code != http.StatusUnauthorized {
		t.Errorf("me after logout = %d, want 401", w.Code)
	}
	// The cookie is gone from the browser, and the session from the database:
	// someone who copied the token has nothing either.
	b.cookies[auth.SessionCookieName] = &http.Cookie{Name: auth.SessionCookieName, Value: token}
	if w := b.get("/api/v1/me"); w.Code != http.StatusUnauthorized {
		t.Errorf("me with the old token = %d, want 401", w.Code)
	}
	// A write from another origin is refused before it does anything.
	if w := b.do("POST", "/auth/logout", map[string]string{"Sec-Fetch-Site": "cross-site"}); w.Code != http.StatusForbidden {
		t.Errorf("cross-site logout = %d, want 403", w.Code)
	}
}

// The role comes from the groups at login and is kept for the life of the
// session: changing the groups later does not change a session that exists, and
// api sessions revoke-user is how that is dealt with (DECISIONS.md 2026-11-08).
func TestNFK10_RolesAreWorkedOutAtLoginAndKeptInTheSession(t *testing.T) {
	he, p := newHandlerEnv(t)
	b := he.browser(t, p)
	he.login(t, b, "")
	he.idp.SetUser(oidctest.User{Subject: oidctest.DefaultUser.Subject, Email: "anna@example.se", Groups: []string{"family"}})
	if w := b.get("/api/v1/me"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"admin"`) {
		t.Errorf("me = %d %s: the session's role changed without a new login", w.Code, w.Body.String())
	}
	// The next login is the one that notices.
	wantRedirect(t, he.login(t, he.browser(t, p), ""), "/login-error?code=no-access")
}
