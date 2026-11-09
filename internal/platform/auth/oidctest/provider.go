// Package oidctest is an OpenID Connect provider in a few hundred lines, for
// tests and for local development (cmd/devoidc). It is not Authelia: it speaks
// the part of the protocol the backend uses (discovery, the authorization code
// flow with PKCE, client_secret_basic, userinfo) and can be told to misbehave in
// the ways a login must survive: a wrong nonce, issuer or audience, a token
// signed with the wrong algorithm, no amr, a userinfo for another subject.
//
// Nothing here may be imported by cmd/api: a test checks that it is not in the
// binary. It signs with a fixed RSA key and approves every login, which is
// exactly what a real provider must never do.
package oidctest

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// Config is the one client the provider knows.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// User is who "logs in". Every authorization request is approved as this user.
type User struct {
	Subject           string
	Email             string
	Name              string
	PreferredUsername string
	Groups            []string
}

// Behavior is how the provider misbehaves. The zero value is a well-behaved
// provider that delivers amr with mfa and the groups from userinfo only, as
// Authelia 4.39 does by default.
type Behavior struct {
	// DiscoveryStatus, when not 0, is what the discovery document answers with.
	DiscoveryStatus int
	// DiscoveryIssuer replaces the issuer the discovery document claims.
	DiscoveryIssuer string
	// TokenStatus, when not 0, makes the token endpoint fail with this status.
	TokenStatus int
	// UserInfoStatus, when not 0, makes userinfo fail with this status.
	UserInfoStatus int
	// DenyAuthorize, when not empty, answers the authorization request with this
	// error code (access_denied, for example) instead of a code.
	DenyAuthorize string

	// Issuer, Audience and Nonce replace the iss, aud and nonce of the ID token.
	// Nonce "" echoes the one from the authorization request.
	Issuer   string
	Audience string
	Nonce    string
	// Alg signs the ID token with another algorithm: "HS256" or "none". Empty is
	// RS256, the only one the backend accepts.
	Alg string
	// Expired makes the ID token expired an hour ago.
	Expired bool
	// OmitAMR leaves amr out. AMR replaces its value, nil being ["pwd","otp","mfa"].
	OmitAMR bool
	AMR     []string
	// UserInfoSubject replaces sub in userinfo.
	UserInfoSubject string
	// GroupsInIDToken also puts email, name and groups in the ID token, as a
	// claims policy would.
	GroupsInIDToken bool
	// OmitUserInfoGroups leaves the groups out of userinfo.
	OmitUserInfoGroups bool
	// OmitEmail leaves the e-mail address out of userinfo.
	OmitEmail bool
}

// Provider is an http.Handler that serves the provider's endpoints.
type Provider struct {
	cfg    Config
	issuer string

	mu       sync.Mutex
	user     User
	behavior Behavior
	grants   map[string]*grant
	tokens   map[string]User // access token -> the user it was issued to
	exchange int
	userinfo int
	calls    map[string]int
	lastReq  TokenRequest
}

type grant struct {
	redirectURI string
	nonce       string
	challenge   string
	user        User
	used        bool
}

// TokenRequest is what the last successful token request carried, so that a
// test can see that the backend sent the PKCE verifier and authenticated.
type TokenRequest struct {
	Verifier    string
	RedirectURI string
	ClientID    string
}

// DefaultUser is a member of the group the backend maps to the admin role.
var DefaultUser = User{
	Subject: "11111111-2222-3333-4444-555555555555", Email: "anna.svensson@example.se",
	Name: "Anna Svensson", PreferredUsername: "anna", Groups: []string{"linochi_erp_users"},
}

// New creates a Provider whose issuer is issuer, a URL without a trailing slash.
func New(issuer string, cfg Config) *Provider {
	return &Provider{
		cfg: cfg, issuer: strings.TrimRight(issuer, "/"), user: DefaultUser,
		grants: map[string]*grant{}, tokens: map[string]User{}, calls: map[string]int{},
	}
}

// Issuer is the URL the provider says it is.
func (p *Provider) Issuer() string { return p.issuer }

// SetUser changes who logs in from now on.
func (p *Provider) SetUser(u User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.user = u
}

// Configure changes how the provider behaves from now on.
func (p *Provider) Configure(f func(*Behavior)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f(&p.behavior)
}

// Exchanges is how many authorization codes have been traded for tokens.
func (p *Provider) Exchanges() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exchange
}

// UserInfoCalls is how many times userinfo was fetched.
func (p *Provider) UserInfoCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.userinfo
}

// Calls is how many requests a path has had, whatever the answer was.
func (p *Provider) Calls(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[path]
}

// LastTokenRequest is the last successful token request.
func (p *Provider) LastTokenRequest() TokenRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastReq
}

// ServeHTTP implements http.Handler.
func (p *Provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.calls[r.URL.Path]++
	p.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/.well-known/openid-configuration":
		p.discovery(w)
	case r.Method == http.MethodGet && r.URL.Path == "/jwks.json":
		p.jwks(w)
	case r.Method == http.MethodGet && r.URL.Path == "/authorize":
		p.authorize(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/token":
		p.token(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/userinfo":
		p.userInfo(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Provider) behaviorNow() Behavior {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.behavior
}

func (p *Provider) discovery(w http.ResponseWriter) {
	b := p.behaviorNow()
	if b.DiscoveryStatus != 0 {
		http.Error(w, "discovery is down", b.DiscoveryStatus)
		return
	}
	issuer := p.issuer
	if b.DiscoveryIssuer != "" {
		issuer = b.DiscoveryIssuer
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                p.issuer + "/authorize",
		"token_endpoint":                        p.issuer + "/token",
		"userinfo_endpoint":                     p.issuer + "/userinfo",
		"jwks_uri":                              p.issuer + "/jwks.json",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256", "HS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		"scopes_supported":                      []string{"openid", "email", "profile", "groups"},
	})
}

func (p *Provider) jwks(w http.ResponseWriter) {
	pub := &signingKey().PublicKey
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": keyID,
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// authorize checks the request the way a provider must (the backend's side of
// the protocol is tested through it) and approves it as the current user.
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	bad := func(msg string) { http.Error(w, "authorization request refused: "+msg, http.StatusBadRequest) }
	switch {
	case q.Get("client_id") != p.cfg.ClientID:
		bad("unknown client_id")
		return
	case q.Get("redirect_uri") != p.cfg.RedirectURL:
		bad("redirect_uri does not match")
		return
	case q.Get("response_type") != "code":
		bad("response_type must be code")
		return
	case q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256":
		bad("PKCE with S256 is required")
		return
	case !slices.Contains(strings.Fields(q.Get("scope")), "openid"):
		bad("scope must contain openid")
		return
	case q.Get("state") == "":
		bad("state is required")
		return
	case q.Get("nonce") == "":
		bad("nonce is required")
		return
	}

	back, err := url.Parse(p.cfg.RedirectURL)
	if err != nil {
		bad("bad configured redirect")
		return
	}
	out := back.Query()
	out.Set("state", q.Get("state"))
	b := p.behaviorNow()
	if b.DenyAuthorize != "" {
		out.Set("error", b.DenyAuthorize)
	} else {
		code := randomString()
		p.mu.Lock()
		p.grants[code] = &grant{redirectURI: p.cfg.RedirectURL, nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), user: p.user}
		p.mu.Unlock()
		out.Set("code", code)
	}
	back.RawQuery = out.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	b := p.behaviorNow()
	if b.TokenStatus != 0 {
		http.Error(w, "token endpoint is down", b.TokenStatus)
		return
	}
	id, secret, ok := r.BasicAuth()
	fail := func(code string) { writeJSON(w, http.StatusBadRequest, map[string]string{"error": code}) }
	if !ok || id != p.cfg.ClientID || subtle.ConstantTimeCompare([]byte(secret), []byte(p.cfg.ClientSecret)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="token"`)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" {
		fail("unsupported_grant_type")
		return
	}

	p.mu.Lock()
	g := p.grants[r.PostForm.Get("code")]
	switch {
	case g == nil || g.used:
		p.mu.Unlock()
		fail("invalid_grant")
		return
	case g.redirectURI != r.PostForm.Get("redirect_uri"):
		p.mu.Unlock()
		fail("invalid_grant")
		return
	case pkceChallenge(r.PostForm.Get("code_verifier")) != g.challenge:
		g.used = true // a failed attempt burns the code, as a careful provider does
		p.mu.Unlock()
		fail("invalid_grant")
		return
	}
	g.used = true
	access := randomString()
	p.tokens[access] = g.user
	p.exchange++
	p.lastReq = TokenRequest{Verifier: r.PostForm.Get("code_verifier"), RedirectURI: g.redirectURI, ClientID: id}
	p.mu.Unlock()

	idToken, err := p.idToken(b, g)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "bearer", "expires_in": 3600, "id_token": idToken,
	})
}

func (p *Provider) idToken(b Behavior, g *grant) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"iss": p.issuer, "sub": g.user.Subject, "aud": []string{p.cfg.ClientID},
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "auth_time": now.Unix(),
		"nonce": g.nonce, "azp": p.cfg.ClientID,
	}
	if b.Issuer != "" {
		claims["iss"] = b.Issuer
	}
	if b.Audience != "" {
		claims["aud"] = []string{b.Audience}
	}
	if b.Nonce != "" {
		claims["nonce"] = b.Nonce
	}
	if b.Expired {
		claims["iat"], claims["exp"] = now.Add(-2*time.Hour).Unix(), now.Add(-time.Hour).Unix()
	}
	if !b.OmitAMR {
		amr := b.AMR
		if amr == nil {
			amr = []string{"pwd", "otp", "mfa"}
		}
		claims["amr"] = amr
	}
	if b.GroupsInIDToken {
		claims["email"], claims["name"], claims["groups"] = g.user.Email, g.user.Name, g.user.Groups
	}
	return sign(b.Alg, claims)
}

func (p *Provider) userInfo(w http.ResponseWriter, r *http.Request) {
	b := p.behaviorNow()
	if b.UserInfoStatus != 0 {
		http.Error(w, "userinfo is down", b.UserInfoStatus)
		return
	}
	access := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if access == r.Header.Get("Authorization") {
		access = strings.TrimPrefix(r.Header.Get("Authorization"), "bearer ")
	}
	p.mu.Lock()
	u, ok := p.tokens[access]
	if ok {
		p.userinfo++
	}
	p.mu.Unlock()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	out := map[string]any{"sub": u.Subject}
	if b.UserInfoSubject != "" {
		out["sub"] = b.UserInfoSubject
	}
	if !b.OmitEmail {
		out["email"] = u.Email
	}
	if u.Name != "" {
		out["name"] = u.Name
	}
	if u.PreferredUsername != "" {
		out["preferred_username"] = u.PreferredUsername
	}
	if !b.OmitUserInfoGroups {
		out["groups"] = u.Groups
	}
	writeJSON(w, http.StatusOK, out)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return b64(sum[:])
}

const keyID = "oidctest-1"

var (
	keyOnce sync.Once
	rsaKey  *rsa.PrivateKey
)

// signingKey is made once per process: generating a key is the slow part.
func signingKey() *rsa.PrivateKey {
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic("oidctest: " + err.Error())
		}
		rsaKey = k
	})
	return rsaKey
}

// sign makes a JWT. "" and "RS256" sign with the provider's key, "HS256" with a
// shared secret that nobody has agreed to, and "none" does not sign at all.
func sign(alg string, claims map[string]any) (string, error) {
	if alg == "" {
		alg = "RS256"
	}
	header, err := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": keyID})
	if err != nil {
		return "", fmt.Errorf("oidctest: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("oidctest: %w", err)
	}
	signingInput := b64(header) + "." + b64(payload)

	var sig []byte
	switch alg {
	case "RS256":
		sum := sha256.Sum256([]byte(signingInput))
		sig, err = rsa.SignPKCS1v15(rand.Reader, signingKey(), crypto.SHA256, sum[:])
		if err != nil {
			return "", fmt.Errorf("oidctest: sign: %w", err)
		}
	case "HS256":
		mac := hmac.New(sha256.New, []byte("a secret nobody agreed on"))
		mac.Write([]byte(signingInput))
		sig = mac.Sum(nil)
	case "none":
	default:
		return "", fmt.Errorf("oidctest: unknown alg %q", alg)
	}
	return signingInput + "." + b64(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func randomString() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("oidctest: " + err.Error())
	}
	return b64(raw)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
