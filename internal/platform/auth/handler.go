package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/api"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// LoginCookieName binds a login in progress to the browser that started it. It
// is a __Host- cookie like the session's, for the same reason, and it lives only
// as long as a login may take.
const LoginCookieName = "__Host-linochi_erp_login"

// HandlerConfig is what the login routes are made of.
type HandlerConfig struct {
	OIDC     *OIDCClient
	Attempts *LoginAttemptStore
	Users    *UserStore
	Sessions *SessionStore
	// AdminGroups are the Authelia groups that give the admin role.
	AdminGroups []string
	Logger      *slog.Logger
	Metrics     *obs.Metrics
	// Now is the clock; time.Now when nil. For tests.
	Now func() time.Time
}

// Handler serves the auth routes of the OpenAPI document (NFK-10, NFK-11):
// /auth/login, /auth/callback, /auth/logout and /api/v1/me. It implements
// api.ServerInterface.
type Handler struct {
	cfg    HandlerConfig
	mw     *SessionMiddleware
	logins *prometheus.CounterVec
}

var _ api.ServerInterface = (*Handler)(nil)

// NewHandler creates a Handler and registers linochi_auth_logins_total.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if cfg.OIDC == nil || cfg.Attempts == nil || cfg.Users == nil || cfg.Sessions == nil || cfg.Metrics == nil {
		return nil, errors.New("auth handler: OIDC, Attempts, Users, Sessions and Metrics are all required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	h := &Handler{
		cfg: cfg,
		mw:  NewSessionMiddleware(cfg.Sessions),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_auth_logins_total",
			Help: "Logins by result: ok, or the reason it was refused (no_role, mfa_missing, attempt_expired, and so on). A rise in anything but ok is what the refused-logins alert watches.",
		}, []string{"result"}),
	}
	if err := cfg.Metrics.Registry.Register(h.logins); err != nil {
		return nil, fmt.Errorf("register login metric: %w", err)
	}
	for _, r := range allReasons {
		h.logins.WithLabelValues(string(r))
	}
	return h, nil
}

// Mount registers the routes of the OpenAPI document on m.
func (h *Handler) Mount(m api.ServeMux) {
	api.HandlerWithOptions(h, api.StdHTTPServerOptions{
		BaseRouter: m,
		// A parameter that does not parse (a repeated one, say) is a bad request
		// like any other, and not the generated code's plain-text answer.
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			httpx.WriteProblem(w, r, httpx.Problem{Code: "invalid-request", Title: "Invalid request", Status: http.StatusBadRequest})
		},
	})
}

// Login starts a login: it stores the attempt, binds it to this browser with a
// cookie and sends the browser to Authelia (GET /auth/login).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request, params api.LoginParams) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")

	// The provider is checked before anything is stored, so that Authelia being
	// down leaves no rows behind.
	if err := h.cfg.OIDC.Ready(ctx); err != nil {
		h.refuse(w, r, err)
		return
	}
	returnTo := "/"
	if params.ReturnTo != nil {
		returnTo = SafeReturnTo(*params.ReturnTo)
	}
	pending, err := h.cfg.Attempts.Begin(ctx, returnTo)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	authorizeURL, err := h.cfg.OIDC.AuthCodeURL(ctx, pending.State, pending.Nonce, pending.Verifier)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	http.SetCookie(w, loginCookie(pending.Binding, int(LoginAttemptTTL/time.Second)))
	http.Redirect(w, r, authorizeURL, http.StatusSeeOther)
}

// LoginCallback finishes a login: it checks everything the provider says and
// only then makes a user and a session (GET /auth/callback). Every way it can
// fail ends in a redirect to the web app's error page with a fixed code.
func (h *Handler) LoginCallback(w http.ResponseWriter, r *http.Request, params api.LoginCallbackParams) {
	ctx := r.Context()
	w.Header().Set("Cache-Control", "no-store")

	// Whatever happens next, the login is over for this browser.
	binding := ""
	if c, err := r.Cookie(LoginCookieName); err == nil {
		binding = c.Value
	}
	http.SetCookie(w, loginCookie("", -1))

	state := deref(params.State)
	if state == "" {
		h.refuse(w, r, &LoginFailure{Reason: ReasonBadCallback, Err: errors.New("no state")})
		return
	}
	// The attempt is consumed before anything else is looked at, so every
	// callback is the last one for its state, good or bad.
	attempt, err := h.cfg.Attempts.Finish(ctx, state, binding)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	if e := deref(params.Error); e != "" {
		reason := ReasonProviderError
		if e == "access_denied" {
			reason = ReasonProviderDenied
		}
		h.refuse(w, r, &LoginFailure{Reason: reason, Err: fmt.Errorf("the provider answered %q", truncate(e, 64))})
		return
	}
	code := deref(params.Code)
	if code == "" {
		h.refuse(w, r, &LoginFailure{Reason: ReasonBadCallback, Err: errors.New("no code")})
		return
	}

	id, err := h.cfg.OIDC.Exchange(ctx, code, attempt.Verifier, attempt.Nonce)
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	roles := RolesFromGroups(id.Groups, h.cfg.AdminGroups)
	if len(roles) == 0 {
		// Authelia let the person in, the app does not. Nothing is made for them.
		h.refuse(w, r, &LoginFailure{Reason: ReasonNoRole, Err: fmt.Errorf("none of %d groups gives a role", len(id.Groups))})
		return
	}
	user, err := h.cfg.Users.UpsertOnLogin(ctx, UserLogin{
		Issuer: id.Issuer, Subject: id.Subject, Email: id.Email, DisplayName: id.DisplayName,
	})
	if err != nil {
		h.refuse(w, r, err)
		return
	}
	token, sess, err := h.cfg.Sessions.Create(ctx, user.ID, roles)
	if err != nil {
		h.refuse(w, r, err)
		return
	}

	http.SetCookie(w, SessionCookie(token, sess.ExpiresAt, h.cfg.Now()))
	h.logins.WithLabelValues(string(ReasonOK)).Inc()
	h.cfg.Logger.InfoContext(ctx, "login", slog.String("user_id", user.ID.String()), slog.String("session_id", sess.ID.String()))
	http.Redirect(w, r, SafeReturnTo(attempt.ReturnTo), http.StatusSeeOther)
}

// Logout ends the session (POST /auth/logout).
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.mw.LogoutHandler().ServeHTTP(w, r)
}

// GetMe says who is logged in (GET /api/v1/me).
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	h.mw.Require(http.HandlerFunc(h.me)).ServeHTTP(w, r)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok {
		httpx.Error(w, r, ErrUnauthenticated)
		return
	}
	user, err := h.cfg.Users.Get(r.Context(), sess.UserID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	body := api.Me{Id: user.ID, Email: user.Email, DisplayName: user.DisplayName, Roles: make([]api.MeRoles, 0, len(sess.Roles))}
	for _, role := range sess.Roles {
		body.Roles = append(body.Roles, api.MeRoles(role))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// refuse ends a login that did not work: it counts it, logs why with the trace
// id and sends the browser to the error page with a fixed code. The reason is
// worked out from the error, and nothing of it is in the response.
func (h *Handler) refuse(w http.ResponseWriter, r *http.Request, err error) {
	reason := reasonOf(err)
	h.logins.WithLabelValues(string(reason)).Inc()

	level := slog.LevelWarn
	if reason == ReasonInternal || reason == ReasonProviderUnavailable || reason == ReasonExchangeFailed ||
		reason == ReasonUserInfoFailed || reason == ReasonProviderError {
		level = slog.LevelError // not the person's doing: someone should look
	}
	h.cfg.Logger.Log(r.Context(), level, "login refused", slog.String("reason", string(reason)), slog.Any("error", err))

	http.Redirect(w, r, LoginErrorPath+"?code="+string(reason.Code()), http.StatusSeeOther)
}

// reasonOf is the reason a login ended with, from the error that ended it.
func reasonOf(err error) Reason {
	var lf *LoginFailure
	switch {
	case errors.As(err, &lf):
		return lf.Reason
	case errors.Is(err, ErrLoginAttemptUnknown):
		return ReasonAttemptUnknown
	case errors.Is(err, ErrLoginAttemptExpired):
		return ReasonAttemptExpired
	case errors.Is(err, ErrLoginBindingMismatch):
		return ReasonBindingMismatch
	case errors.Is(err, ErrTooManyLoginAttempts):
		return ReasonTooManyAttempts
	case errors.Is(err, ErrUserArchived):
		return ReasonUserArchived
	}
	return ReasonInternal
}

// loginCookie is the one place the binding cookie is built, as for the session:
// __Host- needs Secure, Path=/ and no Domain. Lax, and not Strict, because the
// callback is a top-level navigation from Authelia, which Strict would not send
// the cookie on.
func loginCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: LoginCookieName, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
