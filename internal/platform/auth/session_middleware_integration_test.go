//go:build integration

package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
)

type seen struct {
	called  bool
	session auth.Session
	actor   audit.Actor
	hasSess bool
	hasActr bool
}

func (se *sessionEnv) protected(sn *seen) http.Handler {
	return auth.NewSessionMiddleware(se.sessions).Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sn.called = true
		sn.session, sn.hasSess = auth.SessionFrom(r.Context())
		sn.actor, sn.hasActr = audit.ActorFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
}

func request(method, target string, token string, headers map[string]string) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	r.Host = "erp.lab.internal"
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func problemOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, w.Body.String())
	}
	return m
}

func clears(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func TestNFK10_NoCookieGives401AsAProblem(t *testing.T) {
	se := newSessionEnv(t)
	var sn seen
	w := httptest.NewRecorder()
	se.protected(&sn).ServeHTTP(w, request("GET", "/api/v1/me", "", nil))

	if w.Code != http.StatusUnauthorized || sn.called {
		t.Fatalf("status = %d, handler called = %v, want 401 and not called", w.Code, sn.called)
	}
	p := problemOf(t, w)
	if p["type"] != "https://erp.lab.internal/problems/unauthenticated" || p["status"] != float64(401) {
		t.Errorf("problem = %v", p)
	}
	if clears(w) {
		t.Error("a request with no cookie was told to clear one")
	}
}

func TestNFK11_ValidSessionReachesTheHandlerWithTheUserAsActor(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, s := se.login(t, u.ID)

	var sn seen
	w := httptest.NewRecorder()
	se.protected(&sn).ServeHTTP(w, request("GET", "/api/v1/me", token, nil))

	if w.Code != http.StatusNoContent || !sn.called {
		t.Fatalf("status = %d, called = %v", w.Code, sn.called)
	}
	if !sn.hasSess || sn.session.ID != s.ID || sn.session.UserID != u.ID {
		t.Errorf("session in context = %+v (present %v)", sn.session, sn.hasSess)
	}
	// Every write the handler makes is attributed to the person without the
	// handler having to remember to say so (INS-05).
	if !sn.hasActr || sn.actor.Type != audit.ActorUser || sn.actor.ID != u.ID {
		t.Errorf("actor = %+v (present %v), want the logged-in user", sn.actor, sn.hasActr)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q: a logged-in answer must not be cached", cc)
	}
}

func TestNFK11_EndedOrUnknownSessionGives401AndClearsTheCookie(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	idle, _ := se.login(t, u.ID)
	loggedOut, _ := se.login(t, u.ID)
	if err := se.sessions.Logout(context.Background(), loggedOut); err != nil {
		t.Fatal(err)
	}
	unknown, _, _ := auth.NewToken()
	se.clock.Advance(13 * time.Hour)

	for name, token := range map[string]string{"idle": idle, "logged out": loggedOut, "unknown": unknown, "junk": "junk"} {
		t.Run(name, func(t *testing.T) {
			var sn seen
			w := httptest.NewRecorder()
			se.protected(&sn).ServeHTTP(w, request("GET", "/api/v1/me", token, nil))
			if w.Code != http.StatusUnauthorized || sn.called {
				t.Errorf("status = %d, called = %v", w.Code, sn.called)
			}
			if !clears(w) {
				t.Error("the browser was not told to drop a cookie that is no good")
			}
			// All four look the same from outside.
			if p := problemOf(t, w); p["detail"] != nil {
				t.Errorf("the problem says why: %v", p["detail"])
			}
		})
	}
}

func TestNFK11_CrossOriginWritesAreRefusedBeforeTheHandler(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)

	cases := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"cross-site POST", "POST", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site POST from a sibling subdomain", "POST", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"POST with a foreign Origin", "POST", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"DELETE cross-site", "DELETE", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-origin POST", "POST", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"POST with our own Origin", "POST", map[string]string{"Origin": "https://erp.lab.internal"}, http.StatusNoContent},
		{"POST with no browser headers (a tool)", "POST", nil, http.StatusNoContent},
		// Reads are safe by definition; they must not change state, and the
		// session cookie is SameSite=Lax anyway.
		{"cross-site GET", "GET", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sn seen
			w := httptest.NewRecorder()
			se.protected(&sn).ServeHTTP(w, request(tc.method, "/api/v1/things", token, tc.headers))
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			if tc.want == http.StatusForbidden {
				if sn.called {
					t.Error("the handler ran for a refused cross-origin write")
				}
				if p := problemOf(t, w); p["type"] != "https://erp.lab.internal/problems/cross-origin-forbidden" {
					t.Errorf("problem type = %v", p["type"])
				}
				if clears(w) {
					t.Error("a cross-origin refusal logged the user out of their own session")
				}
			}
		})
	}
	// None of that touched the session.
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Errorf("session after the refused requests: %v", err)
	}
}

func TestNFK11_LogoutHandlerEndsTheSessionAndClearsTheCookie(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)

	h := auth.NewSessionMiddleware(se.sessions).LogoutHandler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request("POST", "/auth/logout", token, nil))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if !clears(w) {
		t.Error("logout did not clear the cookie")
	}
	if _, err := se.sessions.Lookup(context.Background(), token); err == nil {
		t.Error("the session still works after logout")
	}
	// Logging out twice, or with no cookie at all, is not an error either.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, request("POST", "/auth/logout", token, nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("second logout status = %d, want 204", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, request("POST", "/auth/logout", "", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf("logout without a cookie status = %d, want 204", w.Code)
	}
}

func TestNFK11_LogoutIsCrossOriginProtectedToo(t *testing.T) {
	se := newSessionEnv(t)
	u := se.newUser(t, "anna")
	token, _ := se.login(t, u.ID)

	w := httptest.NewRecorder()
	auth.NewSessionMiddleware(se.sessions).LogoutHandler().
		ServeHTTP(w, request("POST", "/auth/logout", token, map[string]string{"Sec-Fetch-Site": "cross-site"}))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: another site could log the user out", w.Code)
	}
	if _, err := se.sessions.Lookup(context.Background(), token); err != nil {
		t.Errorf("a refused logout ended the session: %v", err)
	}
}
