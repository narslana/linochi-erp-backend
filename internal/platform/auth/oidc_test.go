package auth_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
)

const callbackURL = "http://localhost:8080/auth/callback"

var providerCfg = oidctest.Config{ClientID: "linochi-erp", ClientSecret: "a-client-secret", RedirectURL: callbackURL}

// clockAt is a clock that stands still until it is moved.
type clockAt struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clockAt) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clockAt) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type oidcFixture struct {
	srv    *oidctest.Server
	client *auth.OIDCClient
	clock  *clockAt
	logs   *bytes.Buffer
}

func newOIDCFixture(t *testing.T, srv *oidctest.Server) *oidcFixture {
	t.Helper()
	f := &oidcFixture{srv: srv, clock: &clockAt{t: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}, logs: &bytes.Buffer{}}
	f.client = auth.NewOIDCClient(auth.OIDCConfig{
		Issuer: srv.Issuer(), ClientID: providerCfg.ClientID, ClientSecret: providerCfg.ClientSecret, RedirectURL: callbackURL,
	}, auth.WithOIDCHTTPClient(srv.Client()), auth.WithOIDCClock(f.clock.now),
		auth.WithOIDCLogger(slog.New(slog.NewTextHandler(f.logs, nil))))
	return f
}

func startedFixture(t *testing.T) *oidcFixture {
	t.Helper()
	return newOIDCFixture(t, oidctest.Start(t, providerCfg))
}

// authorized does what a browser does between the backend and the callback: it
// is sent to the provider with the authorization URL and comes back with a code.
func (f *oidcFixture) authorized(t *testing.T) (code, verifier, nonce string) {
	t.Helper()
	verifier, nonce = oauth2.GenerateVerifier(), "a-nonce-for-this-login"
	loc, err := f.client.AuthCodeURL(context.Background(), "a-state", nonce, verifier)
	if err != nil {
		t.Fatalf("AuthCodeURL: %v", err)
	}
	cb, err := url.Parse(f.srv.Authorize(t, loc))
	if err != nil {
		t.Fatal(err)
	}
	if cb.Query().Get("state") != "a-state" {
		t.Fatalf("callback state = %q", cb.Query().Get("state"))
	}
	return cb.Query().Get("code"), verifier, nonce
}

func reasonOf(t *testing.T, err error) auth.Reason {
	t.Helper()
	var lf *auth.LoginFailure
	if !errors.As(err, &lf) {
		t.Fatalf("err = %v, want a *auth.LoginFailure", err)
	}
	return lf.Reason
}

func TestNFK10_AuthorizationURLCarriesPKCEStateNonceAndTheRightScopes(t *testing.T) {
	f := startedFixture(t)
	loc, err := f.client.AuthCodeURL(context.Background(), "the-state", "the-nonce", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if !strings.HasPrefix(loc, f.srv.Issuer()+"/authorize?") {
		t.Errorf("URL = %q, want the provider's authorization endpoint from discovery", loc)
	}
	want := map[string]string{
		"response_type": "code", "client_id": "linochi-erp", "redirect_uri": callbackURL,
		"state": "the-state", "nonce": "the-nonce", "code_challenge_method": "S256",
		// RFC 7636 appendix B: the S256 challenge of that verifier.
		"code_challenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if scopes := strings.Fields(q.Get("scope")); !slices.Equal(scopes, []string{"openid", "email", "profile", "groups"}) {
		t.Errorf("scope = %v", scopes)
	}
	if strings.Contains(loc, "a-client-secret") || q.Get("code_verifier") != "" {
		t.Error("the authorization URL leaks the client secret or the verifier")
	}
}

func TestNFK10_ExchangeGivesTheIdentityFromTheIDTokenAndUserInfo(t *testing.T) {
	f := startedFixture(t)
	f.srv.SetUser(oidctest.User{
		Subject: "sub-1", Email: "anna@example.se", Name: "Anna Svensson", PreferredUsername: "anna",
		Groups: []string{"linochi_erp_users", "other"},
	})
	code, verifier, nonce := f.authorized(t)

	id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if id.Issuer != f.srv.Issuer() || id.Subject != "sub-1" || id.Email != "anna@example.se" || id.DisplayName != "Anna Svensson" {
		t.Errorf("identity = %+v", id)
	}
	if !slices.Equal(id.Groups, []string{"linochi_erp_users", "other"}) {
		t.Errorf("groups = %v, want the ones from userinfo", id.Groups)
	}
	// The browser is not Authelia's only client: the backend authenticates with
	// its secret and proves it started the login with the PKCE verifier.
	if got := f.srv.LastTokenRequest(); got.Verifier != verifier || got.ClientID != "linochi-erp" || got.RedirectURI != callbackURL {
		t.Errorf("token request = %+v", got)
	}
	if f.srv.UserInfoCalls() != 1 {
		t.Errorf("userinfo was fetched %d times, want once", f.srv.UserInfoCalls())
	}
}

func TestNFK10_ExchangeNeedsTheVerifierThatMatchesTheChallenge(t *testing.T) {
	f := startedFixture(t)
	code, _, nonce := f.authorized(t)
	_, err := f.client.Exchange(context.Background(), code, oauth2.GenerateVerifier(), nonce)
	if got := reasonOf(t, err); got != auth.ReasonExchangeFailed {
		t.Errorf("reason = %s, want %s", got, auth.ReasonExchangeFailed)
	}
}

func TestNFK10_AnAuthorizationCodeWorksOnce(t *testing.T) {
	f := startedFixture(t)
	code, verifier, nonce := f.authorized(t)
	if _, err := f.client.Exchange(context.Background(), code, verifier, nonce); err != nil {
		t.Fatal(err)
	}
	_, err := f.client.Exchange(context.Background(), code, verifier, nonce)
	if got := reasonOf(t, err); got != auth.ReasonExchangeFailed {
		t.Errorf("second use: reason = %s, want %s", got, auth.ReasonExchangeFailed)
	}
}

// Each of these is a token the backend must not accept, and each must be told
// apart in the logs.
func TestNFK10_ExchangeRefusesATokenThatIsNotRightForThisLogin(t *testing.T) {
	cases := []struct {
		name   string
		change func(*oidctest.Behavior)
		want   auth.Reason
	}{
		{"a nonce from another login", func(b *oidctest.Behavior) { b.Nonce = "someone-elses-nonce" }, auth.ReasonNonceMismatch},
		{"another issuer", func(b *oidctest.Behavior) { b.Issuer = "https://evil.example" }, auth.ReasonTokenInvalid},
		{"another audience", func(b *oidctest.Behavior) { b.Audience = "another-client" }, auth.ReasonTokenInvalid},
		{"an expired token", func(b *oidctest.Behavior) { b.Expired = true }, auth.ReasonTokenInvalid},
		{"HS256 instead of RS256", func(b *oidctest.Behavior) { b.Alg = "HS256" }, auth.ReasonTokenInvalid},
		{"alg none", func(b *oidctest.Behavior) { b.Alg = "none" }, auth.ReasonTokenInvalid},
		{"a userinfo for another subject", func(b *oidctest.Behavior) { b.UserInfoSubject = "someone-else" }, auth.ReasonSubjectMismatch},
		{"a userinfo that fails", func(b *oidctest.Behavior) { b.UserInfoStatus = http.StatusInternalServerError }, auth.ReasonUserInfoFailed},
		{"a token endpoint that fails", func(b *oidctest.Behavior) { b.TokenStatus = http.StatusBadGateway }, auth.ReasonExchangeFailed},
		{"no e-mail address", func(b *oidctest.Behavior) { b.OmitEmail = true }, auth.ReasonClaimsMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := startedFixture(t)
			code, verifier, nonce := f.authorized(t)
			f.srv.Configure(tc.change)
			id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
			if err == nil {
				t.Fatalf("accepted: %+v", id)
			}
			if got := reasonOf(t, err); got != tc.want {
				t.Errorf("reason = %s, want %s (err: %v)", got, tc.want, err)
			}
			if id.Subject != "" || id.Email != "" {
				t.Errorf("an identity came back with the error: %+v", id)
			}
		})
	}
}

// NFK-10: two-factor only. Authelia's policy is the gate, and amr is a second
// lock on the same door when the claim is there.
func TestNFK10_ExchangeChecksAMRForTwoFactorWhenItIsDelivered(t *testing.T) {
	cases := []struct {
		name     string
		change   func(*oidctest.Behavior)
		wantErr  auth.Reason
		wantWarn bool
	}{
		{"mfa", func(*oidctest.Behavior) {}, "", false},
		{"mfa among others", func(b *oidctest.Behavior) { b.AMR = []string{"pwd", "hwk", "mfa"} }, "", false},
		{"password only", func(b *oidctest.Behavior) { b.AMR = []string{"pwd"} }, auth.ReasonMFARequired, false},
		{"empty", func(b *oidctest.Behavior) { b.AMR = []string{} }, auth.ReasonMFARequired, false},
		{"one factor of two is not mfa", func(b *oidctest.Behavior) { b.AMR = []string{"pwd", "otp"} }, auth.ReasonMFARequired, false},
		{"not delivered", func(b *oidctest.Behavior) { b.OmitAMR = true }, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := startedFixture(t)
			f.srv.Configure(tc.change)
			code, verifier, nonce := f.authorized(t)
			_, err := f.client.Exchange(context.Background(), code, verifier, nonce)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("a login without two factors was accepted")
				}
				if got := reasonOf(t, err); got != tc.wantErr {
					t.Errorf("reason = %s, want %s", got, tc.wantErr)
				}
			}
			// Without the claim the app relies on Authelia's two_factor policy, and
			// says so in the log so that it is not a silent assumption.
			if warned := strings.Contains(f.logs.String(), "amr"); warned != tc.wantWarn {
				t.Errorf("amr warning logged = %v, want %v; logs: %s", warned, tc.wantWarn, f.logs.String())
			}
		})
	}
}

func TestNFK10_GroupsComeFromUserInfoAndFallBackToTheIDToken(t *testing.T) {
	t.Run("userinfo wins", func(t *testing.T) {
		f := startedFixture(t)
		f.srv.Configure(func(b *oidctest.Behavior) { b.GroupsInIDToken = true })
		f.srv.SetUser(oidctest.User{Subject: "s", Email: "a@example.se", Groups: []string{"admins"}})
		code, verifier, nonce := f.authorized(t)
		id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
		if err != nil || !slices.Equal(id.Groups, []string{"admins"}) {
			t.Errorf("groups = %v, err = %v", id.Groups, err)
		}
	})
	t.Run("a claims policy puts them in the ID token only", func(t *testing.T) {
		f := startedFixture(t)
		f.srv.Configure(func(b *oidctest.Behavior) { b.GroupsInIDToken, b.OmitUserInfoGroups = true, true })
		f.srv.SetUser(oidctest.User{Subject: "s", Email: "a@example.se", Groups: []string{"linochi_erp_users"}})
		code, verifier, nonce := f.authorized(t)
		id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
		if err != nil || !slices.Equal(id.Groups, []string{"linochi_erp_users"}) {
			t.Errorf("groups = %v, err = %v", id.Groups, err)
		}
	})
	t.Run("nowhere is no groups, and not an error", func(t *testing.T) {
		f := startedFixture(t)
		f.srv.Configure(func(b *oidctest.Behavior) { b.OmitUserInfoGroups = true })
		code, verifier, nonce := f.authorized(t)
		id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
		if err != nil || len(id.Groups) != 0 {
			t.Errorf("groups = %v, err = %v: the role check is the caller's, and an empty list gives no role", id.Groups, err)
		}
	})
}

func TestNFK10_DisplayNameFallsBackToTheUsername(t *testing.T) {
	f := startedFixture(t)
	f.srv.SetUser(oidctest.User{Subject: "s", Email: "a@example.se", PreferredUsername: "anna", Groups: []string{"admins"}})
	code, verifier, nonce := f.authorized(t)
	id, err := f.client.Exchange(context.Background(), code, verifier, nonce)
	if err != nil || id.DisplayName != "anna" {
		t.Errorf("display name = %q, err = %v", id.DisplayName, err)
	}
}

// Authelia being down must not stop this app from starting, so the first
// contact is at the first login, and a restart of Authelia is not a restart of us.
func TestNFK10_ProviderDownAtFirstUseIsRetriedAndNeedsNoRestart(t *testing.T) {
	srv := oidctest.Start(t, providerCfg)
	srv.Configure(func(b *oidctest.Behavior) { b.DiscoveryStatus = http.StatusServiceUnavailable })
	f := newOIDCFixture(t, srv)

	_, err := f.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier())
	if got := reasonOf(t, err); got != auth.ReasonProviderUnavailable {
		t.Fatalf("reason = %s, want %s", got, auth.ReasonProviderUnavailable)
	}

	// It is back, but a request right after a failure does not go to it again:
	// a provider that is struggling is not hammered by every login attempt.
	srv.Configure(func(b *oidctest.Behavior) { b.DiscoveryStatus = 0 })
	if _, err := f.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier()); err == nil {
		t.Error("discovery was retried straight away")
	}
	if n := srv.Calls("/.well-known/openid-configuration"); n != 1 {
		t.Errorf("discovery requests = %d, want 1 until the retry delay has passed", n)
	}

	f.clock.advance(auth.DiscoveryRetryDelay)
	if _, err := f.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier()); err != nil {
		t.Fatalf("after the delay: %v", err)
	}
	// And once it works it is remembered: discovery is not repeated per login.
	for range 3 {
		if _, err := f.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier()); err != nil {
			t.Fatal(err)
		}
	}
	if n := srv.Calls("/.well-known/openid-configuration"); n != 2 {
		t.Errorf("discovery requests = %d, want 2 in all", n)
	}
}

func TestNFK10_DiscoveryThatClaimsAnotherIssuerIsRefused(t *testing.T) {
	// A server at the configured address that says it is somebody else is not
	// used: its tokens would be checked against the wrong issuer.
	srv := oidctest.Start(t, providerCfg)
	srv.Configure(func(b *oidctest.Behavior) { b.DiscoveryIssuer = "https://evil.example" })
	f := newOIDCFixture(t, srv)
	_, err := f.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier())
	if got := reasonOf(t, err); got != auth.ReasonProviderUnavailable {
		t.Errorf("reason = %s, err = %v", got, err)
	}
}

// NFK-12 at the level of the client: a provider with a certificate from a CA the
// client was not given is not trusted. The same at the level of the whole
// process is in cmd/api.
func TestNFK12_ClientTrustsOnlyTheCAItWasGiven(t *testing.T) {
	srv := oidctest.StartTLS(t, providerCfg)

	trusting := newOIDCFixture(t, srv) // srv.Client() trusts the server's certificate
	if _, err := trusting.client.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier()); err != nil {
		t.Fatalf("with the CA: %v", err)
	}

	plain := auth.NewOIDCClient(auth.OIDCConfig{
		Issuer: srv.Issuer(), ClientID: "linochi-erp", ClientSecret: "x", RedirectURL: callbackURL,
	}) // the default client: system roots only
	_, err := plain.AuthCodeURL(context.Background(), "s", "n", oauth2.GenerateVerifier())
	if got := reasonOf(t, err); got != auth.ReasonProviderUnavailable {
		t.Fatalf("without the CA: reason = %s, err = %v", got, err)
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("the error does not say it was the certificate: %v", err)
	}
}

func TestNFK10_NoSecretOrTokenReachesTheLogs(t *testing.T) {
	f := startedFixture(t)
	f.srv.Configure(func(b *oidctest.Behavior) { b.OmitAMR = true; b.UserInfoSubject = "someone-else" })
	code, verifier, nonce := f.authorized(t)
	_, _ = f.client.Exchange(context.Background(), code, verifier, nonce)
	for _, secret := range []string{providerCfg.ClientSecret, code, verifier, nonce, "anna.svensson@example.se"} {
		if strings.Contains(f.logs.String(), secret) {
			t.Errorf("the logs contain %q:\n%s", secret, f.logs.String())
		}
	}
}
