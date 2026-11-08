package auth

import (
	"errors"
	"net/http"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

// SessionCookieName has the __Host- prefix, which makes the browser accept the
// cookie only if it is Secure, has Path=/ and has no Domain. A sibling
// subdomain, or anything on plain HTTP, cannot set or overwrite it.
const SessionCookieName = "__Host-linochi_erp_session"

// errCrossOrigin is what http.CrossOriginProtection refusing a request becomes.
var errCrossOrigin error = &sessionError{httpx.Problem{
	Code: "cross-origin-forbidden", Title: "Cross-origin request refused", Status: http.StatusForbidden,
}}

// SessionCookie is the cookie that carries the token. It is the same in every
// environment: nothing here is switched off for development. It lives until the
// absolute limit, so the browser drops it when the session can no longer be
// used (the idle limit is the server's to enforce). A session already past its
// limit gets a cookie that deletes itself.
func SessionCookie(token string, expiresAt, now time.Time) *http.Cookie {
	maxAge := int(expiresAt.Sub(now) / time.Second)
	if maxAge <= 0 {
		maxAge = -1 // zero would mean "no Max-Age", a cookie that outlives the session
	}
	return sessionCookie(token, maxAge)
}

// ClearSessionCookie tells the browser to drop the cookie. It keeps every
// attribute of the real one: a __Host- cookie can only be replaced by a
// Set-Cookie that meets the same rules.
func ClearSessionCookie() *http.Cookie {
	return sessionCookie("", -1)
}

// sessionCookie is the one place the cookie is built, so that the real one and
// the clearing one cannot drift apart. __Host- needs Secure, Path=/ and no
// Domain; HttpOnly keeps it from scripts; Lax keeps it off cross-site POSTs.
func sessionCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// SessionMiddleware guards the routes that need a logged-in user (/api and
// /auth/logout, NFK-10).
type SessionMiddleware struct {
	store *SessionStore
	// cross rejects cross-origin writes. It is the standard library's: it looks
	// at Sec-Fetch-Site and, where a browser does not send that, at Origin
	// against Host. Reads are never refused, so nothing must change state on GET.
	cross *http.CrossOriginProtection
}

// NewSessionMiddleware creates a SessionMiddleware.
func NewSessionMiddleware(store *SessionStore) *SessionMiddleware {
	return &SessionMiddleware{store: store, cross: http.NewCrossOriginProtection()}
}

// Require lets a request through only with a valid session, and puts the
// session and the user as audit actor in its context, so that every write the
// handler makes is attributed to the person (INS-05). Without one the answer is
// 401. A cookie that is no good is cleared, so the browser stops sending it. A
// cross-origin write is refused with 403 before anything else happens, and
// leaves the session alone.
func (m *SessionMiddleware) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if m.cross.Check(r) != nil {
			httpx.Error(w, r, errCrossOrigin)
			return
		}
		c, err := r.Cookie(SessionCookieName)
		if err != nil {
			httpx.Error(w, r, ErrUnauthenticated)
			return
		}
		sess, err := m.store.Lookup(r.Context(), c.Value)
		if err != nil {
			if errors.Is(err, ErrUnauthenticated) {
				http.SetCookie(w, ClearSessionCookie())
			}
			httpx.Error(w, r, err)
			return
		}
		ctx := WithSession(r.Context(), sess)
		ctx = audit.WithActor(ctx, audit.UserActor(sess.UserID))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LogoutHandler ends the session of the request and clears the cookie. It does
// not need a valid session: logging out with an ended one, or twice, works and
// answers 204 like the first time. It is protected against cross-origin
// requests, or another site could log the user out. The route's method (POST)
// is set where it is registered.
func (m *SessionMiddleware) LogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if m.cross.Check(r) != nil {
			httpx.Error(w, r, errCrossOrigin)
			return
		}
		if c, err := r.Cookie(SessionCookieName); err == nil {
			if err := m.store.Logout(r.Context(), c.Value); err != nil {
				httpx.Error(w, r, err)
				return
			}
		}
		http.SetCookie(w, ClearSessionCookie())
		w.WriteHeader(http.StatusNoContent)
	})
}
