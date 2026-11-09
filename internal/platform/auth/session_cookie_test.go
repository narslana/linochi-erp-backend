package auth_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
)

// The cookie is the same in every environment. There is no flag that weakens it
// here: a development mode, if one is ever needed, is decided outside this
// function (B6) and never by changing these attributes in production.
func TestNFK11_SessionCookieHasTheHostPrefixAndAllTheAttributes(t *testing.T) {
	token, _, _ := auth.NewToken()
	expires := t0.Add(7 * 24 * time.Hour)
	c := auth.SessionCookie(token, expires, t0.Add(time.Hour))

	if !strings.HasPrefix(c.Name, "__Host-") {
		t.Errorf("cookie name %q lacks the __Host- prefix", c.Name)
	}
	if c.Name != auth.SessionCookieName {
		t.Errorf("name = %q, want SessionCookieName", c.Name)
	}
	if c.Value != token {
		t.Error("the cookie does not carry the token")
	}
	if !c.Secure || !c.HttpOnly {
		t.Errorf("Secure=%v HttpOnly=%v, want both", c.Secure, c.HttpOnly)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", c.SameSite)
	}
	// __Host- requires Path=/ and no Domain: the browser ignores the cookie otherwise.
	if c.Path != "/" || c.Domain != "" {
		t.Errorf("Path=%q Domain=%q, want / and empty", c.Path, c.Domain)
	}
	if want := int((7*24*time.Hour - time.Hour) / time.Second); c.MaxAge != want {
		t.Errorf("MaxAge = %d, want %d: the cookie ends with the absolute limit", c.MaxAge, want)
	}
	if err := c.Valid(); err != nil {
		t.Errorf("cookie is not valid: %v", err)
	}
}

func TestNFK11_SessionCookieNeverOutlivesTheSession(t *testing.T) {
	token, _, _ := auth.NewToken()
	// A session already past its limit must not get a cookie with a negative or
	// zero MaxAge that the browser reads as "delete" or "session cookie".
	c := auth.SessionCookie(token, t0, t0.Add(time.Hour))
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d for an ended session, want a deleting (negative) value", c.MaxAge)
	}
}

func TestNFK11_ClearingTheCookieKeepsTheAttributesThatMakeItDeletable(t *testing.T) {
	c := auth.ClearSessionCookie()
	if c.Name != auth.SessionCookieName || c.Value != "" {
		t.Errorf("name=%q value=%q", c.Name, c.Value)
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative", c.MaxAge)
	}
	// A __Host- cookie can only be overwritten by a Set-Cookie that meets the
	// same rules, so the clearing one has to be Secure with Path=/ as well.
	if !c.Secure || !c.HttpOnly || c.Path != "/" || c.Domain != "" || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("clearing cookie attributes: %+v", c)
	}
}
