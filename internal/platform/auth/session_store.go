package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db/dbgen"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

const (
	sessionEntity = "session"
	// logoutAction is the audit action of a logout. Login is audit.Create.
	logoutAction = "logout"
	// sessionsRevokedAction is recorded on the user when all their sessions end.
	sessionsRevokedAction = "sessions_revoked"

	// CleanupInterval is how often ended sessions are deleted. An ended session
	// is refused by Lookup whether or not its row is still there, so this only
	// keeps the table small.
	CleanupInterval = time.Hour
)

// sessionError is a typed domain error that httpx maps to RFC 9457.
type sessionError struct{ problem httpx.Problem }

func (e *sessionError) Error() string          { return e.problem.Title }
func (e *sessionError) Problem() httpx.Problem { return e.problem }

var (
	// ErrUnauthenticated means there is no usable session: no cookie, an unknown
	// or ended session, or a user who has been archived. The caller is never
	// told which, so a probe learns nothing about what exists.
	ErrUnauthenticated error = &sessionError{httpx.Problem{
		Code: "unauthenticated", Title: "Authentication required", Status: http.StatusUnauthorized,
	}}
	// ErrUserArchived means a login for a user who has been archived.
	ErrUserArchived error = &sessionError{httpx.Problem{
		Code: "account-archived", Title: "The account is archived", Status: http.StatusForbidden,
	}}
)

type sessionKey struct{}

// WithSession returns a context carrying the session of the request.
func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

// SessionFrom returns the session the middleware put in the context.
func SessionFrom(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(sessionKey{}).(Session)
	return s, ok
}

// SessionStore keeps sessions in Postgres, so that every replica sees the same
// ones (NFK-02). Creating a session, ending one and ending all of a user's are
// audited in the same transaction (INS-05).
type SessionStore struct {
	pool   *pgxpool.Pool
	audit  *audit.Recorder
	now    func() time.Time
	logger *slog.Logger
	tracer trace.Tracer

	checks  *prometheus.CounterVec
	deleted *prometheus.CounterVec
	cleanup *prometheus.CounterVec
}

// SessionOption configures a SessionStore.
type SessionOption func(*SessionStore)

// WithClock replaces the clock. For tests, which move time by hand.
func WithClock(now func() time.Time) SessionOption { return func(s *SessionStore) { s.now = now } }

// WithLogger sets the logger. The default is slog's default logger.
func WithLogger(l *slog.Logger) SessionOption { return func(s *SessionStore) { s.logger = l } }

// WithTracerProvider sets where spans go. The default creates none.
func WithTracerProvider(tp trace.TracerProvider) SessionOption {
	return func(s *SessionStore) { s.tracer = tp.Tracer("linochi/auth") }
}

// NewSessionStore creates a SessionStore and registers its metrics:
// linochi_auth_session_checks_total, linochi_auth_sessions_deleted_total and
// linochi_auth_session_cleanup_runs_total.
func NewSessionStore(pool *pgxpool.Pool, rec *audit.Recorder, metrics *obs.Metrics, opts ...SessionOption) (*SessionStore, error) {
	s := &SessionStore{
		pool: pool, audit: rec,
		now:    time.Now,
		logger: slog.Default(),
		tracer: noop.NewTracerProvider().Tracer("linochi/auth"),
		checks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_auth_session_checks_total",
			Help: "Session checks by result: ok, malformed, unknown, idle_expired, absolute_expired, user_archived or error.",
		}, []string{"result"}),
		deleted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_auth_sessions_deleted_total",
			Help: "Sessions deleted, by reason: logout, revoked or cleanup.",
		}, []string{"reason"}),
		cleanup: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_auth_session_cleanup_runs_total",
			Help: "Runs of the session cleanup by result: ok or error.",
		}, []string{"result"}),
	}
	for _, o := range opts {
		o(s)
	}
	for _, c := range []prometheus.Collector{s.checks, s.deleted, s.cleanup} {
		if err := metrics.Registry.Register(c); err != nil {
			return nil, fmt.Errorf("register session metric: %w", err)
		}
	}
	return s, nil
}

// Create logs a user in: it makes a session and returns the token, which is
// shown to the browser once and never stored. roles are the roles worked out
// from the Authelia groups at this login. The user row is locked while the
// session is made, so that archiving the user at the same moment cannot leave a
// session behind: either the archive commits first and this fails with
// ErrUserArchived, or the archive waits and ends the session it then finds.
func (s *SessionStore) Create(ctx context.Context, userID uuid.UUID, roles []string) (string, Session, error) {
	if len(roles) == 0 {
		return "", Session{}, errors.New("create session: a session needs at least one role")
	}
	ctx = audit.WithActor(ctx, audit.UserActor(userID))
	token, hash, err := NewToken()
	if err != nil {
		return "", Session{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", Session{}, fmt.Errorf("new session id: %w", err)
	}
	now := s.now()

	var sess Session
	err = db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		q := tx.Queries()
		user, err := q.LockActiveUserForSession(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if user.ArchivedAt != nil {
			return ErrUserArchived
		}
		row, err := q.InsertSession(ctx, dbgen.InsertSessionParams{
			ID: id, UserID: userID, TokenHash: hash, Roles: roles,
			CreatedAt: now, ExpiresAt: AbsoluteExpiry(now),
		})
		if err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		sess = sessionFromRow(row)
		return s.audit.Record(ctx, tx, audit.Entry{
			Module: auditModule, EntityType: sessionEntity, EntityID: sess.ID,
			Action: audit.Create, After: sess,
		})
	})
	if err != nil {
		return "", Session{}, fmt.Errorf("create session for user %s: %w", userID, err)
	}
	return token, sess, nil
}

// Lookup returns the session for a token from a cookie, or ErrUnauthenticated.
// It also counts as use: last_seen_at is moved forward, but at most once per
// TouchInterval, and never the absolute limit. A failure to write that is
// logged and does not fail the request: the session is valid, and only the idle
// clock is a minute behind.
func (s *SessionStore) Lookup(ctx context.Context, token string) (Session, error) {
	ctx, span := s.tracer.Start(ctx, "auth.session.lookup")
	defer span.End()

	result := "error"
	defer func() {
		s.checks.WithLabelValues(result).Inc()
		span.SetAttributes(attribute.String("session.result", result))
	}()

	if !ValidTokenFormat(token) {
		result = "malformed"
		return Session{}, ErrUnauthenticated
	}
	q := dbgen.New(s.pool)
	row, err := q.GetSessionByTokenHash(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		result = "unknown"
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		span.SetStatus(codes.Error, "lookup failed")
		return Session{}, fmt.Errorf("look up session: %w", err)
	}
	sess := sessionFromRow(dbgen.Session{
		ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
		UserID: row.UserID, Roles: row.Roles, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt,
	})
	if row.UserArchivedAt != nil {
		result = "user_archived"
		return Session{}, ErrUnauthenticated
	}

	now := s.now()
	switch sess.Status(now) {
	case StatusIdleExpired:
		result = "idle_expired"
		return Session{}, ErrUnauthenticated
	case StatusAbsoluteExpired:
		result = "absolute_expired"
		return Session{}, ErrUnauthenticated
	case StatusActive:
	}

	if sess.NeedsTouch(now) {
		if _, err := q.TouchSession(ctx, dbgen.TouchSessionParams{ID: sess.ID, LastSeenAt: now}); err != nil {
			s.logger.WarnContext(ctx, "touching the session failed", slog.Any("error", err))
		} else {
			sess = sess.Touched(now)
		}
	}
	result = "ok"
	return sess, nil
}

// Logout ends the session of a token. A token that matches nothing is not an
// error: the browser's cookie is cleared either way, and a repeated logout
// should not look like a failure.
func (s *SessionStore) Logout(ctx context.Context, token string) error {
	if !ValidTokenFormat(token) {
		return nil
	}
	return s.inTx(ctx, "logout", func(ctx context.Context, tx *db.Tx) (int, error) {
		row, err := tx.Queries().DeleteSessionByTokenHash(ctx, HashToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		if err != nil {
			return 0, fmt.Errorf("delete session: %w", err)
		}
		sess := sessionFromRow(row)
		// The user is the actor, taken from the row: the caller may have no
		// request context worth trusting by now.
		ctx = audit.WithActor(ctx, audit.UserActor(sess.UserID))
		err = s.audit.Record(ctx, tx, audit.Entry{
			Module: auditModule, EntityType: sessionEntity, EntityID: sess.ID,
			Action: logoutAction, Before: sess,
		})
		return 1, err
	})
}

// RevokeAllForUser ends every session of one user at once, for example when
// access is removed in Authelia or a device is lost. It returns how many ended.
// The actor is the one in the context, and without one nothing is deleted.
func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := s.inTx(ctx, "revoked", func(ctx context.Context, tx *db.Tx) (int, error) {
		var err error
		n, err = endUserSessions(ctx, s.audit, tx, userID)
		return n, err
	})
	if err != nil {
		return 0, fmt.Errorf("revoke sessions of user %s: %w", userID, err)
	}
	return n, nil
}

// inTx runs fn in a transaction and counts the sessions it ended in the metric
// once the commit has happened.
func (s *SessionStore) inTx(ctx context.Context, reason string, fn func(context.Context, *db.Tx) (int, error)) error {
	return db.InTx(ctx, s.pool, func(tx *db.Tx) error {
		n, err := fn(ctx, tx)
		if err != nil {
			return err
		}
		if n > 0 {
			tx.AfterCommit(func() { s.deleted.WithLabelValues(reason).Add(float64(n)) })
		}
		return nil
	})
}

// endUserSessions deletes every session of the user and records that on the
// user. It is shared with UserStore.Archive, which must end the sessions in the
// same transaction as the archive. Nothing is recorded when there were none.
func endUserSessions(ctx context.Context, rec *audit.Recorder, tx *db.Tx, userID uuid.UUID) (int, error) {
	rows, err := tx.Queries().DeleteSessionsForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("delete sessions: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	err = rec.Record(ctx, tx, audit.Entry{
		Module: auditModule, EntityType: auditEntity, EntityID: userID,
		Action: sessionsRevokedAction, After: map[string]any{"count": len(rows)},
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// Cleanup deletes the sessions that have ended, by the same two limits as
// Lookup. It is idempotent: any number of replicas may run it at once and each
// row is deleted exactly once.
func (s *SessionStore) Cleanup(ctx context.Context) (n int64, err error) {
	ctx, span := s.tracer.Start(ctx, "auth.session.cleanup")
	defer span.End()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
			span.SetStatus(codes.Error, "cleanup failed")
		}
		s.cleanup.WithLabelValues(result).Inc()
		span.SetAttributes(attribute.Int64("session.deleted", n))
	}()

	now := s.now()
	n, err = dbgen.New(s.pool).DeleteEndedSessions(ctx, dbgen.DeleteEndedSessionsParams{
		Now: now, IdleCutoff: now.Add(-IdleTimeout),
	})
	if err != nil {
		return 0, fmt.Errorf("delete ended sessions: %w", err)
	}
	if n > 0 {
		s.deleted.WithLabelValues("cleanup").Add(float64(n))
		s.logger.InfoContext(ctx, "ended sessions deleted", slog.Int64("count", n))
	}
	return n, nil
}

// RunCleanup runs Cleanup now and then every interval until ctx is cancelled.
// A failed run is logged and tried again at the next tick.
func (s *SessionStore) RunCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.Cleanup(ctx); err != nil && ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "session cleanup failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sessionFromRow drops the token hash on purpose: nothing outside the store has
// a use for it, and a Session is what ends up in audit entries.
func sessionFromRow(r dbgen.Session) Session {
	return Session{
		ID: r.ID, UserID: r.UserID, Roles: r.Roles,
		CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt, ExpiresAt: r.ExpiresAt,
	}
}
