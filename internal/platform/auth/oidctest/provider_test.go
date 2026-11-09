package oidctest_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth/oidctest"
)

var cfg = oidctest.Config{ClientID: "linochi-erp", ClientSecret: "s3cret", RedirectURL: "http://localhost:8080/auth/callback"}

func authorizeURL(base string, mod func(url.Values)) string {
	q := url.Values{
		"client_id": {cfg.ClientID}, "redirect_uri": {cfg.RedirectURL}, "response_type": {"code"},
		"scope": {"openid email profile groups"}, "state": {"st"}, "nonce": {"no"},
		"code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}, "code_challenge_method": {"S256"},
	}
	if mod != nil {
		mod(q)
	}
	return base + "/authorize?" + q.Encode()
}

func status(t *testing.T, c *http.Client, method, target string, form url.Values, basic bool) int {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic {
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	}
	c2 := *c
	c2.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c2.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// The tests of the backend lean on this provider to be strict, or they would
// pass against a client that sends no PKCE at all.
func TestNFK10_TestProviderRefusesWhatARealOneWould(t *testing.T) {
	s := oidctest.Start(t, cfg)
	c := s.Client()

	if got := status(t, c, http.MethodGet, authorizeURL(s.Issuer(), nil), nil, false); got != http.StatusFound {
		t.Fatalf("a good authorization request: %d, want 302", got)
	}
	for name, mod := range map[string]func(url.Values){
		"no PKCE":          func(q url.Values) { q.Del("code_challenge") },
		"plain PKCE":       func(q url.Values) { q.Set("code_challenge_method", "plain") },
		"no state":         func(q url.Values) { q.Del("state") },
		"no nonce":         func(q url.Values) { q.Del("nonce") },
		"no openid scope":  func(q url.Values) { q.Set("scope", "email") },
		"another redirect": func(q url.Values) { q.Set("redirect_uri", "https://evil.example/cb") },
		"another client":   func(q url.Values) { q.Set("client_id", "other") },
		"implicit flow":    func(q url.Values) { q.Set("response_type", "token") },
	} {
		if got := status(t, c, http.MethodGet, authorizeURL(s.Issuer(), mod), nil, false); got != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, got)
		}
	}
}

func TestNFK10_TestProviderChecksTheTokenRequest(t *testing.T) {
	s := oidctest.Start(t, cfg)
	c := s.Client()
	// The challenge in authorizeURL is the S256 of this verifier (RFC 7636, appendix B).
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	callback := s.Authorize(t, authorizeURL(s.Issuer(), nil))
	u, err := url.Parse(callback)
	if err != nil || u.Query().Get("state") != "st" || u.Query().Get("code") == "" {
		t.Fatalf("callback = %q, err = %v", callback, err)
	}
	code := u.Query().Get("code")
	form := func(v string) url.Values {
		return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {cfg.RedirectURL}, "code_verifier": {v}}
	}

	if got := status(t, c, http.MethodPost, s.Issuer()+"/token", form(verifier), false); got != http.StatusUnauthorized {
		t.Errorf("no client authentication: %d, want 401", got)
	}
	if got := status(t, c, http.MethodPost, s.Issuer()+"/token", form("wrong"), true); got != http.StatusBadRequest {
		t.Errorf("wrong verifier: %d, want 400", got)
	}
	// The wrong verifier burned the code.
	if got := status(t, c, http.MethodPost, s.Issuer()+"/token", form(verifier), true); got != http.StatusBadRequest {
		t.Errorf("a code that failed once: %d, want 400", got)
	}

	callback = s.Authorize(t, authorizeURL(s.Issuer(), nil))
	u, _ = url.Parse(callback)
	code = u.Query().Get("code")
	if got := status(t, c, http.MethodPost, s.Issuer()+"/token", form(verifier), true); got != http.StatusOK {
		t.Fatalf("a good token request: %d, want 200", got)
	}
	if got := status(t, c, http.MethodPost, s.Issuer()+"/token", form(verifier), true); got != http.StatusBadRequest {
		t.Errorf("a code used twice: %d, want 400", got)
	}
	if s.Exchanges() != 1 || s.LastTokenRequest().Verifier != verifier {
		t.Errorf("exchanges = %d, last verifier = %q", s.Exchanges(), s.LastTokenRequest().Verifier)
	}
}
