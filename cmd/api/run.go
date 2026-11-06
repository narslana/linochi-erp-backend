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

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
)

// run is main without the process globals, so that it can be tested. It
// returns the exit code.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) > 1 && args[1] == "version" {
		_, _ = fmt.Fprintf(stdout, "%s · %s\n", version, commit)
		return 0
	}

	cfg, err := config.Load(getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: cfg.SlogLevel()}))
	logger.InfoContext(ctx, "starting", slog.String("version", version), slog.String("commit", commit), slog.String("config", cfg.String()))

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		logger.ErrorContext(ctx, "listen failed", slog.String("addr", cfg.HTTPAddr), slog.Any("error", err))
		return 1
	}

	// Routes arrive with their steps: /healthz in B2, /auth and /api in B6.
	mux := http.NewServeMux()

	if err := serve(ctx, logger, ln, mux, cfg.ShutdownTimeout); err != nil {
		logger.ErrorContext(ctx, "server stopped with error", slog.Any("error", err))
		return 1
	}
	logger.InfoContext(context.WithoutCancel(ctx), "stopped")
	return 0
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
