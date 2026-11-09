package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
)

const (
	// LoginAttemptTTL is how long a person has to finish a login at Authelia,
	// including the second factor.
	LoginAttemptTTL = 10 * time.Minute
	// DefaultMaxOpenAttempts caps the attempts that have not expired. /auth/login
	// needs no session, so without a cap anyone could fill the table. A real
	// login takes seconds, and this app has a handful of users.
	DefaultMaxOpenAttempts = 5000
)

var (
	// ErrLoginAttemptUnknown means no attempt matches the state: it never
	// existed, was already used or was deleted by the cleanup.
	ErrLoginAttemptUnknown = errors.New("login attempt: unknown or already used")
	// ErrLoginAttemptExpired means the attempt was older than LoginAttemptTTL.
	ErrLoginAttemptExpired = errors.New("login attempt: expired")
	// ErrLoginBindingMismatch means the callback came from a browser that did not
	// start the login (login CSRF).
	ErrLoginBindingMismatch = errors.New("login attempt: started in another browser")
	// ErrTooManyLoginAttempts means the cap on open attempts is reached.
	ErrTooManyLoginAttempts = errors.New("login attempt: too many open attempts")
)

// PendingLogin is what a started login hands to the caller: the values that go
// to Authelia (State, Nonce, and the challenge of Verifier) and the one that
// stays with the browser (Binding). Only hashes of State and Binding are stored.
type PendingLogin struct {
	State    string
	Binding  string
	Nonce    string
	Verifier string
}

// FinishedLogin is what a login attempt held, handed back once.
type FinishedLogin struct {
	Nonce    string
	Verifier string
	ReturnTo string
}

// LoginAttemptStore keeps the logins in progress in Postgres, so that the
// callback can land on any replica (NFK-02) and a state works exactly once.
type LoginAttemptStore struct {
	pool    *pgxpool.Pool
	now     func() time.Time
	maxOpen int64
	logger  *slog.Logger
}

// LoginAttemptOption configures a LoginAttemptStore.
type LoginAttemptOption func(*LoginAttemptStore)

// WithAttemptClock replaces the clock. For tests, which move time by hand.
func WithAttemptClock(now func() time.Time) LoginAttemptOption {
	return func(s *LoginAttemptStore) { s.now = now }
}

// WithMaxOpenAttempts replaces DefaultMaxOpenAttempts.
func WithMaxOpenAttempts(n int) LoginAttemptOption {
	return func(s *LoginAttemptStore) { s.maxOpen = int64(n) }
}

// WithAttemptLogger sets the logger. The default is slog's default logger.
func WithAttemptLogger(l *slog.Logger) LoginAttemptOption {
	return func(s *LoginAttemptStore) { s.logger = l }
}

// NewLoginAttemptStore creates a LoginAttemptStore.
func NewLoginAttemptStore(pool *pgxpool.Pool, opts ...LoginAttemptOption) *LoginAttemptStore {
	s := &LoginAttemptStore{pool: pool, now: time.Now, maxOpen: DefaultMaxOpenAttempts, logger: slog.Default()}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Begin stores a login in progress. returnTo must already have been checked
// with SafeReturnTo. It fails with ErrTooManyLoginAttempts when the cap is
// reached.
func (s *LoginAttemptStore) Begin(ctx context.Context, returnTo string) (PendingLogin, error) {
	state, stateHash, err := NewToken()
	if err != nil {
		return PendingLogin{}, err
	}
	binding, bindingHash, err := NewToken()
	if err != nil {
		return PendingLogin{}, err
	}
	nonce, _, err := NewToken()
	if err != nil {
		return PendingLogin{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return PendingLogin{}, fmt.Errorf("new login attempt id: %w", err)
	}
	p := PendingLogin{State: state, Binding: binding, Nonce: nonce, Verifier: oauth2.GenerateVerifier()}

	now := s.now()
	_, err = dbgen.New(s.pool).InsertLoginAttempt(ctx, dbgen.InsertLoginAttemptParams{
		ID: id, StateHash: stateHash, BindingHash: bindingHash, Nonce: p.Nonce, CodeVerifier: p.Verifier,
		ReturnTo: returnTo, CreatedAt: now, ExpiresAt: now.Add(LoginAttemptTTL), MaxOpen: s.maxOpen,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingLogin{}, ErrTooManyLoginAttempts
	}
	if err != nil {
		return PendingLogin{}, fmt.Errorf("store login attempt: %w", err)
	}
	return p, nil
}

// Finish consumes the attempt of a state: the row is deleted and returned in one
// statement, so the state works once, also when two replicas get the callback at
// the same moment. The attempt is consumed whatever else is wrong with the
// callback (expired, started in another browser), so a refused callback cannot
// be tried again. binding is the value of the login cookie.
func (s *LoginAttemptStore) Finish(ctx context.Context, state, binding string) (FinishedLogin, error) {
	if !ValidTokenFormat(state) {
		return FinishedLogin{}, ErrLoginAttemptUnknown
	}
	row, err := dbgen.New(s.pool).ConsumeLoginAttempt(ctx, HashToken(state))
	if errors.Is(err, pgx.ErrNoRows) {
		return FinishedLogin{}, ErrLoginAttemptUnknown
	}
	if err != nil {
		return FinishedLogin{}, fmt.Errorf("consume login attempt: %w", err)
	}
	if !s.now().Before(row.ExpiresAt) {
		return FinishedLogin{}, ErrLoginAttemptExpired
	}
	if subtle.ConstantTimeCompare(HashToken(binding), row.BindingHash) != 1 {
		return FinishedLogin{}, ErrLoginBindingMismatch
	}
	return FinishedLogin{Nonce: row.Nonce, Verifier: row.CodeVerifier, ReturnTo: row.ReturnTo}, nil
}

// Cleanup deletes the attempts that have expired. It is idempotent: any number
// of replicas may run it at once. An expired attempt is refused by Finish
// whether or not its row is still there, so this only keeps the table small.
func (s *LoginAttemptStore) Cleanup(ctx context.Context) (int64, error) {
	n, err := dbgen.New(s.pool).DeleteExpiredLoginAttempts(ctx, s.now())
	if err != nil {
		return 0, fmt.Errorf("delete expired login attempts: %w", err)
	}
	if n > 0 {
		s.logger.InfoContext(ctx, "expired login attempts deleted", slog.Int64("count", n))
	}
	return n, nil
}

// RunCleanup runs Cleanup now and then every interval until ctx is cancelled. A
// failed run is logged and tried again at the next tick.
func (s *LoginAttemptStore) RunCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.Cleanup(ctx); err != nil && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "login attempt cleanup failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
