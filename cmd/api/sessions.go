package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace/noop"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

const (
	sessionsUsage = "usage: api sessions revoke-user <email|user-id>"
	// revokeActor is who the audit log says ended the sessions: the operator's
	// command, not a person, since there is no login behind `kubectl exec`.
	revokeActor = "cli-revoke-user"
)

// runSessions is the `api sessions` subcommand, run with `kubectl exec` in a
// running pod. revoke-user ends every session of one user at once: when access
// is removed in Authelia (the roles live in the session, so the person would
// otherwise stay logged in for up to 7 days) or when a device is lost. It runs
// as the app role with DATABASE_URL, which is all it needs, and the change is
// audited like any other (INS-05). Exit codes follow `api migrate`: 2 for a
// usage or configuration error, 1 for anything that went wrong at run time.
func runSessions(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) < 3 {
		_, _ = fmt.Fprintln(stderr, sessionsUsage)
		return 2
	}
	if args[2] != "revoke-user" {
		_, _ = fmt.Fprintf(stderr, "unknown sessions action %q\n%s\n", args[2], sessionsUsage)
		return 2
	}
	rest := args[3:]
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		_, _ = fmt.Fprintln(stderr, sessionsUsage)
		return 2
	}
	who := strings.TrimSpace(rest[0])

	// The app role's URL and nothing else: this command has to work when the
	// login's settings are what is wrong. There is no fallback to the owner's
	// URL, which is for migrations only.
	cfg, err := config.LoadOperator(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	logger := obs.NewLogger(stderr, cfg.SlogLevel())
	metrics := obs.NewMetrics(version, commit)
	pool, err := db.Open(ctx, db.Options{
		URL: cfg.DatabaseURL, MaxConns: 2, Metrics: metrics,
		Tracer: noop.NewTracerProvider(), AppName: "linochi-erp-backend-cli",
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer pool.Close()
	rec, err := audit.NewRecorder(logger, metrics)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	sessions, err := auth.NewSessionStore(pool, rec, metrics, auth.WithLogger(logger))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	userID, err := resolveUser(ctx, auth.NewUserStore(pool, rec), who)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	ctx = audit.WithActor(ctx, audit.SystemActor(revokeActor))
	n, err := sessions.RevokeAllForUser(ctx, userID)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	noun := "sessions"
	if n == 1 {
		noun = "session"
	}
	// The id, never the address: it is enough to find the person, and personal
	// data stays out of what a command prints and a terminal keeps.
	_, _ = fmt.Fprintf(stdout, "ended %d %s for user %s\n", n, noun, userID)
	return 0
}

// resolveUser turns what the operator typed into one user id. A UUID is taken as
// the id. Anything else is an e-mail address, and an address that matches more
// than one user is refused, with the ids listed so that the operator can choose:
// ending the wrong person's sessions is cheap to undo, but guessing is not the
// kind of thing this command should do.
func resolveUser(ctx context.Context, users *auth.UserStore, who string) (uuid.UUID, error) {
	if id, err := uuid.Parse(who); err == nil {
		if _, err := users.Get(ctx, id); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return uuid.Nil, errors.New("no such user")
			}
			return uuid.Nil, fmt.Errorf("look up user: %w", err)
		}
		return id, nil
	}
	found, err := users.FindActiveByEmail(ctx, who)
	if err != nil {
		return uuid.Nil, err //nolint:wrapcheck // already wrapped by auth
	}
	switch len(found) {
	case 0:
		return uuid.Nil, errors.New("no such user")
	case 1:
		return found[0].ID, nil
	}
	var b strings.Builder
	b.WriteString("more than one user has that address; run the command again with one of these ids:")
	for _, u := range found {
		b.WriteString("\n  " + u.ID.String())
	}
	return uuid.Nil, errors.New(b.String())
}
