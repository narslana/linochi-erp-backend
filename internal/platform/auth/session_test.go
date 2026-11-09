package auth_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
)

var t0 = time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)

func newSession(createdAt, lastSeen time.Time) auth.Session {
	return auth.Session{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     uuid.Must(uuid.NewV7()),
		Roles:      []string{"admin"},
		CreatedAt:  createdAt,
		LastSeenAt: lastSeen,
		ExpiresAt:  auth.AbsoluteExpiry(createdAt),
	}
}

func TestNFK11_ExpiresAfter12hIdle(t *testing.T) {
	s := newSession(t0, t0)
	cases := []struct {
		name string
		now  time.Time
		want auth.Status
	}{
		{"just created", t0, auth.StatusActive},
		{"one second before the limit", t0.Add(12*time.Hour - time.Second), auth.StatusActive},
		{"exactly 12 hours idle", t0.Add(12 * time.Hour), auth.StatusIdleExpired},
		{"after 12 hours idle", t0.Add(12*time.Hour + time.Second), auth.StatusIdleExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Status(tc.now); got != tc.want {
				t.Errorf("Status(%s) = %v, want %v", tc.now.Sub(t0), got, tc.want)
			}
		})
	}
}

func TestNFK11_ExpiresAfter7dAbsolute(t *testing.T) {
	// Used every hour for the whole week: never idle, but the week still ends it.
	last := t0.Add(7*24*time.Hour - time.Hour)
	s := newSession(t0, last)
	cases := []struct {
		name string
		now  time.Time
		want auth.Status
	}{
		{"one second before 7 days", t0.Add(7*24*time.Hour - time.Second), auth.StatusActive},
		{"exactly 7 days", t0.Add(7 * 24 * time.Hour), auth.StatusAbsoluteExpired},
		{"after 7 days, still active a minute ago", t0.Add(7*24*time.Hour + time.Minute), auth.StatusAbsoluteExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.Status(tc.now); got != tc.want {
				t.Errorf("Status(%s) = %v, want %v", tc.now.Sub(t0), got, tc.want)
			}
		})
	}
}

func TestNFK11_AbsoluteLimitWinsWhenBothHaveRunOut(t *testing.T) {
	s := newSession(t0, t0)
	if got := s.Status(t0.Add(8 * 24 * time.Hour)); got != auth.StatusAbsoluteExpired {
		t.Errorf("Status = %v, want StatusAbsoluteExpired (the reason that cannot be fixed by activity)", got)
	}
}

func TestNFK11_ActivityDoesNotExtendPast7d(t *testing.T) {
	if got := auth.AbsoluteExpiry(t0); !got.Equal(t0.Add(7 * 24 * time.Hour)) {
		t.Errorf("AbsoluteExpiry = %s, want created + 7 days", got)
	}
	// A touch moves last_seen_at only. expires_at is never recomputed.
	s := newSession(t0, t0)
	touched := s.Touched(t0.Add(11 * time.Hour))
	if !touched.ExpiresAt.Equal(s.ExpiresAt) {
		t.Errorf("Touched changed ExpiresAt from %s to %s", s.ExpiresAt, touched.ExpiresAt)
	}
	if !touched.LastSeenAt.Equal(t0.Add(11 * time.Hour)) {
		t.Errorf("Touched LastSeenAt = %s, want the touch time", touched.LastSeenAt)
	}
}

func TestNFK11_TouchIsRateLimitedToOncePerMinute(t *testing.T) {
	s := newSession(t0, t0)
	if s.NeedsTouch(t0.Add(auth.TouchInterval - time.Second)) {
		t.Error("NeedsTouch true inside the interval: every request would write to the database")
	}
	if !s.NeedsTouch(t0.Add(auth.TouchInterval)) {
		t.Error("NeedsTouch false at the interval")
	}
	// The slack is one interval, so a session is never kept alive past the limit
	// by more than a minute of skipped writes, and never cut short by more.
	if auth.TouchInterval > time.Minute {
		t.Errorf("TouchInterval = %s, the plan says at most one minute", auth.TouchInterval)
	}
}

func TestNFK11_OnlyTokenHashIsStored(t *testing.T) {
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not unpadded base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("token carries %d random bytes, want 32", len(raw))
	}
	want := sha256.Sum256([]byte(token))
	if !bytes.Equal(hash, want[:]) {
		t.Error("hash is not the SHA-256 of the token as the browser sends it")
	}
	if bytes.Contains(hash, []byte(token)) || bytes.Equal(hash, raw) {
		t.Error("the hash must not contain the token")
	}
	if !bytes.Equal(auth.HashToken(token), hash) {
		t.Error("HashToken does not give the hash NewToken returned")
	}
}

func TestNFK11_TokensAreUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for range 1000 {
		token, _, err := auth.NewToken()
		if err != nil {
			t.Fatal(err)
		}
		if _, dup := seen[token]; dup {
			t.Fatal("two tokens were equal")
		}
		seen[token] = struct{}{}
	}
}

func TestNFK11_MalformedTokenIsRejectedBeforeTheDatabase(t *testing.T) {
	for _, bad := range []string{"", "short", "not base64 !!!", string(bytes.Repeat([]byte("a"), 500))} {
		if auth.ValidTokenFormat(bad) {
			t.Errorf("ValidTokenFormat(%q) = true", bad)
		}
	}
	token, _, _ := auth.NewToken()
	if !auth.ValidTokenFormat(token) {
		t.Error("a token from NewToken must be valid")
	}
}

func TestNFK11_SessionHasNoFieldThatCouldCarryTheTokenHash(t *testing.T) {
	// audit entries are made from structs through JSON (internal/audit). The
	// hash in the audit log could never be redacted, so the domain type has to
	// be one that cannot carry it.
	raw, err := json.Marshal(newSession(t0, t0))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for name := range fields {
		if strings.Contains(strings.ToLower(name), "hash") || strings.Contains(strings.ToLower(name), "token") {
			t.Errorf("a session serialises a field %q, which could put the token or its hash in the audit log", name)
		}
	}
	for _, want := range []string{"id", "user_id", "roles", "created_at", "last_seen_at", "expires_at"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("a session does not serialise %q: audit entries would be missing it", want)
		}
	}
}
