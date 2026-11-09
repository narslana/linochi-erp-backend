package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
)

const (
	// DiscoveryRetryDelay is how long after a failed discovery the next login
	// waits before the provider is asked again. A provider that is struggling is
	// not hammered by every login attempt, and one that is back is used again
	// without a restart of the app.
	DiscoveryRetryDelay = 5 * time.Second

	// oidcTimeout bounds every call to the provider.
	oidcTimeout = 10 * time.Second

	// mfaValue is what Authelia puts in amr for a login with more than one
	// factor (RFC 8176).
	mfaValue = "mfa"
)

// OIDCConfig is what the client needs to know about the provider and itself.
type OIDCConfig struct {
	// Issuer is the provider's URL, for example https://auth.lab.internal. The
	// discovery document must claim exactly this issuer.
	Issuer       string
	ClientID     string
	ClientSecret string
	// RedirectURL is the callback registered with the provider, matched exactly.
	RedirectURL string
}

// Identity is who the provider says logged in. It is worked out from the ID
// token and userinfo and is only returned when every check has passed.
type Identity struct {
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
	Groups      []string
}

// OIDCClient is the authorization code flow with PKCE against one provider
// (NFK-10). It is made without contacting the provider: the first login does
// that, so the app starts while Authelia is down.
type OIDCClient struct {
	cfg    OIDCConfig
	http   *http.Client
	logger *slog.Logger
	now    func() time.Time

	mu      sync.Mutex
	state   *providerState // set once discovery has worked
	failed  error          // the last discovery failure, until retryAt
	retryAt time.Time
}

type providerState struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// OIDCOption configures an OIDCClient.
type OIDCOption func(*oidcOptions)

type oidcOptions struct {
	http   *http.Client
	tracer trace.TracerProvider
	logger *slog.Logger
	now    func() time.Time
}

// WithOIDCHTTPClient replaces the HTTP client. In tests it trusts a self-signed
// certificate. In the cluster the default is right: the system roots, with the
// internal CA added through SSL_CERT_DIR or SSL_CERT_FILE (NFK-12).
func WithOIDCHTTPClient(c *http.Client) OIDCOption { return func(o *oidcOptions) { o.http = c } }

// WithOIDCTracerProvider makes the calls to the provider spans of the request.
func WithOIDCTracerProvider(tp trace.TracerProvider) OIDCOption {
	return func(o *oidcOptions) { o.tracer = tp }
}

// WithOIDCLogger sets the logger. The default is slog's default logger.
func WithOIDCLogger(l *slog.Logger) OIDCOption { return func(o *oidcOptions) { o.logger = l } }

// WithOIDCClock replaces the clock the retry delay is measured by. For tests.
func WithOIDCClock(now func() time.Time) OIDCOption { return func(o *oidcOptions) { o.now = now } }

// NewOIDCClient creates an OIDCClient. It does not contact the provider.
func NewOIDCClient(cfg OIDCConfig, opts ...OIDCOption) *OIDCClient {
	o := oidcOptions{logger: slog.Default(), now: time.Now}
	for _, f := range opts {
		f(&o)
	}
	c := o.http
	if c == nil {
		c = &http.Client{
			Timeout: oidcTimeout,
			// The provider's endpoints are where they say they are: a redirect
			// from one is not followed with our credentials.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if o.tracer != nil {
		wrapped := *c
		base := wrapped.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		wrapped.Transport = otelhttp.NewTransport(base, otelhttp.WithTracerProvider(o.tracer))
		c = &wrapped
	}
	return &OIDCClient{cfg: cfg, http: c, logger: o.logger, now: o.now}
}

// discover returns the provider, asking it the first time and after a failure
// once DiscoveryRetryDelay has passed. Only a success is remembered.
func (c *OIDCClient) discover(ctx context.Context) (*providerState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state != nil {
		return c.state, nil
	}
	if c.failed != nil && c.now().Before(c.retryAt) {
		return nil, c.failed
	}

	ctx, cancel := context.WithTimeout(oidc.ClientContext(ctx, c.http), oidcTimeout)
	defer cancel()
	p, err := oidc.NewProvider(ctx, c.cfg.Issuer)
	if err != nil {
		c.failed = &LoginFailure{Reason: ReasonProviderUnavailable, Err: fmt.Errorf("discover %s: %w", c.cfg.Issuer, err)}
		c.retryAt = c.now().Add(DiscoveryRetryDelay)
		return nil, c.failed
	}
	ep := p.Endpoint()
	ep.AuthStyle = oauth2.AuthStyleInHeader // client_secret_basic, as registered
	c.state = &providerState{
		provider: p,
		// RS256 only: a token signed with anything else, HMAC with a key anyone
		// could guess among them, is not one the provider made.
		verifier: p.Verifier(&oidc.Config{ClientID: c.cfg.ClientID, SupportedSigningAlgs: []string{oidc.RS256}}),
		oauth: &oauth2.Config{
			ClientID: c.cfg.ClientID, ClientSecret: c.cfg.ClientSecret, Endpoint: ep, RedirectURL: c.cfg.RedirectURL,
			Scopes: []string{oidc.ScopeOpenID, "email", "profile", "groups"},
		},
	}
	c.failed = nil
	return c.state, nil
}

// Ready reports whether the provider can be used now, asking it if it has not
// been asked yet. A login checks this before it stores anything, so that a
// provider that is down leaves no rows behind.
func (c *OIDCClient) Ready(ctx context.Context) error {
	_, err := c.discover(ctx)
	return err
}

// AuthCodeURL is where the browser is sent to log in: the provider's
// authorization endpoint with the state, the nonce and the PKCE challenge of
// verifier.
func (c *OIDCClient) AuthCodeURL(ctx context.Context, state, nonce, verifier string) (string, error) {
	st, err := c.discover(ctx)
	if err != nil {
		return "", err
	}
	return st.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), nil
}

// Exchange trades the authorization code for tokens and checks them. Every
// failure is a *LoginFailure. The tokens are used here and dropped: they are
// never stored and never logged (the session is the app's own, NFK-11).
func (c *OIDCClient) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	st, err := c.discover(ctx)
	if err != nil {
		return Identity{}, err
	}
	fail := func(r Reason, format string, args ...any) (Identity, error) {
		return Identity{}, &LoginFailure{Reason: r, Err: fmt.Errorf(format, args...)}
	}

	ctx, cancel := context.WithTimeout(oidc.ClientContext(ctx, c.http), oidcTimeout)
	defer cancel()
	tok, err := st.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return fail(ReasonExchangeFailed, "exchange the code: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return fail(ReasonTokenInvalid, "the token response has no id_token")
	}
	idt, err := st.verifier.Verify(ctx, raw)
	if err != nil {
		return fail(ReasonTokenInvalid, "verify the ID token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(nonce)) != 1 {
		return fail(ReasonNonceMismatch, "the ID token was made for another login")
	}
	var idClaims map[string]json.RawMessage
	if err := idt.Claims(&idClaims); err != nil {
		return fail(ReasonTokenInvalid, "read the ID token claims: %w", err)
	}

	// Two-factor only (NFK-10). Authelia's two_factor policy is the gate; amr is
	// a second lock when the provider says how the person logged in.
	if rawAMR, ok := idClaims["amr"]; ok {
		var amr []string
		if err := json.Unmarshal(rawAMR, &amr); err != nil {
			return fail(ReasonTokenInvalid, "amr is not a list of strings")
		}
		if !slices.Contains(amr, mfaValue) {
			return fail(ReasonMFARequired, "amr has no %q", mfaValue)
		}
	} else {
		c.logger.WarnContext(ctx, "the ID token has no amr claim: relying on the provider's two-factor policy")
	}

	ui, err := st.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil {
		return fail(ReasonUserInfoFailed, "fetch userinfo: %w", err)
	}
	if ui.Subject != idt.Subject {
		return fail(ReasonSubjectMismatch, "userinfo describes another subject than the ID token")
	}
	var uiClaims map[string]json.RawMessage
	if err := ui.Claims(&uiClaims); err != nil {
		return fail(ReasonUserInfoFailed, "read the userinfo claims: %w", err)
	}

	id := Identity{Issuer: idt.Issuer, Subject: idt.Subject}
	// Userinfo is where Authelia 4.39 puts these by default. A claims policy may
	// put them in the ID token instead, so that is the fallback.
	if id.Email, err = stringClaim("email", uiClaims, idClaims); err != nil || id.Email == "" {
		return fail(ReasonClaimsMissing, "the provider gave no e-mail address")
	}
	if id.DisplayName, err = stringClaim("name", uiClaims, idClaims); err != nil {
		return fail(ReasonClaimsMissing, "name is not a string")
	}
	if id.DisplayName == "" {
		if id.DisplayName, err = stringClaim("preferred_username", uiClaims, idClaims); err != nil {
			return fail(ReasonClaimsMissing, "preferred_username is not a string")
		}
	}
	if id.Groups, err = stringsClaim("groups", uiClaims, idClaims); err != nil {
		return fail(ReasonClaimsMissing, "groups is not a list of strings")
	}
	return id, nil
}

// stringClaim is the first of the sources that has the claim.
func stringClaim(name string, sources ...map[string]json.RawMessage) (string, error) {
	for _, m := range sources {
		if raw, ok := m[name]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return "", errors.New("not a string")
			}
			return s, nil
		}
	}
	return "", nil
}

// stringsClaim is the first of the sources that has the claim, even when it is
// empty: userinfo saying "no groups" is not overruled by an older ID token.
func stringsClaim(name string, sources ...map[string]json.RawMessage) ([]string, error) {
	for _, m := range sources {
		if raw, ok := m[name]; ok {
			var s []string
			if err := json.Unmarshal(raw, &s); err != nil {
				return nil, errors.New("not a list of strings")
			}
			return s, nil
		}
	}
	return nil, nil
}
