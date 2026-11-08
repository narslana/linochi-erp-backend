package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// The session limits (NFK-11). They are constants and not configuration: the
// requirement is a fixed rule, and a value read from the environment could
// differ between two replicas that share a session (NFK-02).
const (
	// IdleTimeout is how long a session may go unused.
	IdleTimeout = 12 * time.Hour
	// AbsoluteTimeout is the longest a session lives, however active it is.
	AbsoluteTimeout = 7 * 24 * time.Hour
	// TouchInterval is how seldom last_seen_at is written. A session in use is
	// not updated on every request, so reads do not turn into writes. The cost
	// is that a session can be cut short by up to one interval, never extended
	// past the limit.
	TouchInterval = time.Minute

	tokenBytes = 32
	// tokenLen is the length of a token as the browser sends it: 32 bytes as
	// unpadded base64url.
	tokenLen = 43
)

// Status says whether a session may be used.
type Status int

const (
	// StatusActive means the session may be used.
	StatusActive Status = iota
	// StatusIdleExpired means the session was unused for IdleTimeout.
	StatusIdleExpired
	// StatusAbsoluteExpired means the session reached AbsoluteTimeout.
	StatusAbsoluteExpired
)

func (s Status) String() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusIdleExpired:
		return "idle_expired"
	case StatusAbsoluteExpired:
		return "absolute_expired"
	}
	return "unknown"
}

// Session is a logged-in browser. It deliberately has no token and no token
// hash: audit entries are made from structs through JSON, and a hash that got
// into the audit log could never be redacted out of it (the log is append-only).
type Session struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Roles      []string  `json:"roles"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	// ExpiresAt is the absolute limit. It is set once, when the session is
	// created, and activity never moves it.
	ExpiresAt time.Time `json:"expires_at"`
}

// AbsoluteExpiry is when a session created at createdAt ends, whatever happens.
func AbsoluteExpiry(createdAt time.Time) time.Time {
	return createdAt.Add(AbsoluteTimeout)
}

// Status works out whether the session may be used at now. The absolute limit
// is checked first: it is the reason that no amount of activity can fix.
func (s Session) Status(now time.Time) Status {
	if !now.Before(s.ExpiresAt) {
		return StatusAbsoluteExpired
	}
	if !now.Before(s.LastSeenAt.Add(IdleTimeout)) {
		return StatusIdleExpired
	}
	return StatusActive
}

// NeedsTouch reports whether last_seen_at is old enough to be written again.
func (s Session) NeedsTouch(now time.Time) bool {
	return !now.Before(s.LastSeenAt.Add(TouchInterval))
}

// Touched returns the session as it is after being used at now. Only
// LastSeenAt moves.
func (s Session) Touched(now time.Time) Session {
	s.LastSeenAt = now
	return s
}

// NewToken makes a session token and the hash that is stored in its place. The
// token is 32 random bytes from crypto/rand, so SHA-256 is enough: it is not a
// password and there is nothing to guess or to stretch.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken is what the database stores and looks up.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// ValidTokenFormat reports whether token could have come from NewToken. A cookie
// that does not look like one is refused before any database call, so junk
// cookies from a scanner cost nothing.
func ValidTokenFormat(token string) bool {
	if len(token) != tokenLen {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(raw) == tokenBytes
}
