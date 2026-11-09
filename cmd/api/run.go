package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/audit"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/auth"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// run is main without the process globals, so that it can be tested. It
// returns the exit code.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		switch args[1] {
		case "version":
			_, _ = fmt.Fprintf(stdout, "%s · %s\n", version, commit)
			return 0
		case "migrate":
			return runMigrate(ctx, args, getenv, stdout, stderr)
		case "sessions":
			return runSessions(ctx, args, getenv, stdout, stderr)
		}
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	logger := obs.NewLogger(stderr, cfg.SlogLevel())
	logger.InfoContext(ctx, "starting", slog.String("version", version), slog.String("commit", commit), slog.String("config", cfg.String()))

	tp, err := obs.NewTracerProvider(ctx, obs.TracingConfig{Endpoint: cfg.OTLPEndpoint, Version: version, PodName: cfg.PodName})
	if err != nil {
		logger.ErrorContext(ctx, "tracing setup failed", slog.Any("error", err))
		return 1
	}
	defer func() {
		// Flush buffered spans. The parent context is cancelled by now.
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(flushCtx); err != nil {
			logger.ErrorContext(flushCtx, "flushing traces failed", slog.Any("error", err))
		}
	}()

	metrics := obs.NewMetrics(version, commit)
	// The pool connects lazily: the app starts while the database is down or the
	// schema is behind, and /readyz stays red until that is fixed (NFK-02).
	pool, err := db.Open(ctx, db.Options{
		URL: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns, MinConns: cfg.DBMinConns,
		MaxConnLifetime: cfg.DBMaxConnLife, Metrics: metrics, Tracer: tp,
	})
	if err != nil {
		logger.ErrorContext(ctx, "database setup failed", slog.Any("error", err))
		return 1
	}
	defer pool.Close()

	rec, err := audit.NewRecorder(logger, metrics)
	if err != nil {
		logger.ErrorContext(ctx, "audit setup failed", slog.Any("error", err))
		return 1
	}
	sessions, err := auth.NewSessionStore(pool, rec, metrics, auth.WithLogger(logger), auth.WithTracerProvider(tp))
	if err != nil {
		logger.ErrorContext(ctx, "session setup failed", slog.Any("error", err))
		return 1
	}
	// Ended sessions are deleted in the background, on every replica. The delete
	// is idempotent, so two replicas doing it at once is fine. A run that fails,
	// for example while the database is down or the schema is behind, is logged
	// and tried again: it must never stop the app from starting (NFK-02).
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		sessions.RunCleanup(cleanupCtx, auth.CleanupInterval)
	}()
	// Registered after pool.Close, so it runs before it: the cleanup must not be
	// left with a closed pool.
	defer func() {
		stopCleanup()
		<-cleanupDone
	}()

	health := &httpx.Health{}
	health.AddCheck("database", db.ReadinessCheck(pool))
	public, internal := newHandlers(logger, metrics, health, tp)

	var lc net.ListenConfig
	pubLn, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		logger.ErrorContext(ctx, "listen failed", slog.String("addr", cfg.HTTPAddr), slog.Any("error", err))
		return 1
	}
	intLn, err := lc.Listen(ctx, "tcp", cfg.InternalAddr)
	if err != nil {
		_ = pubLn.Close()
		logger.ErrorContext(ctx, "listen failed", slog.String("addr", cfg.InternalAddr), slog.Any("error", err))
		return 1
	}

	err = runServers(ctx, logger, health, cfg.ShutdownDelay, cfg.ShutdownTimeout,
		server{"public", pubLn, public}, server{"internal", intLn, internal})
	if err != nil {
		logger.ErrorContext(ctx, "server stopped with error", slog.Any("error", err))
		return 1
	}
	logger.InfoContext(context.WithoutCancel(ctx), "stopped")
	return 0
}

// newHandlers builds the two routers. The public one carries /api and /auth
// (B6 and later); the internal one has no ingress route and carries the probes
// and /metrics. API-key routes join it in phase 6.
func newHandlers(logger *slog.Logger, metrics *obs.Metrics, health *httpx.Health, tp trace.TracerProvider) (public, internal http.Handler) {
	pub := httpx.NewRouter(httpx.RouterOptions{Server: "public", Logger: logger, Metrics: metrics, Tracer: tp})

	in := httpx.NewRouter(httpx.RouterOptions{Server: "internal", Logger: logger, Metrics: metrics, Tracer: tp})
	in.Handle("GET /healthz", health.Healthz())
	in.Handle("GET /readyz", health.Readyz())
	in.Handle("GET /metrics", metrics.Handler())
	return pub, in
}

type server struct {
	name    string
	ln      net.Listener
	handler http.Handler
}

// runServers serves all servers until ctx is cancelled. Then it drains: /readyz
// turns 503 at once, and the listeners stay open for delay so that Traefik and
// the endpoints controller stop sending traffic (NFK-02). After that every
// server shuts down gracefully, each bounded by timeout. If one server fails on
// its own, the others are stopped.
func runServers(ctx context.Context, logger *slog.Logger, health *httpx.Health, delay, timeout time.Duration, servers ...server) error {
	srvCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	defer stop()

	go func() {
		select {
		case <-ctx.Done():
		case <-srvCtx.Done():
			return
		}
		health.SetDraining()
		logger.InfoContext(srvCtx, "draining", slog.String("delay", delay.String()))
		select {
		case <-time.After(delay):
		case <-srvCtx.Done():
		}
		stop()
	}()

	errc := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			err := serve(srvCtx, logger.With(slog.String("server", s.name)), s.ln, s.handler, timeout)
			if err != nil {
				stop() // take the others down with it
			}
			errc <- err
		}()
	}
	var errs []error
	for range servers {
		if err := <-errc; err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// serve serves handler on ln until ctx is cancelled, then lets in-flight
// requests finish for at most shutdownTimeout (NFK-02: rolling updates).
func serve(ctx context.Context, logger *slog.Logger, ln net.Listener, handler http.Handler, shutdownTimeout time.Duration) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	errc := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "listening", slog.String("addr", ln.Addr().String()))
		errc <- srv.Serve(ln)
	}()

	select {
	case err := <-errc:
		// Serve only returns on its own if something is wrong.
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.InfoContext(context.WithoutCancel(ctx), "shutting down", slog.String("timeout", shutdownTimeout.String()))
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
