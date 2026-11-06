package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// Options configures Open.
type Options struct {
	URL             string
	MaxConns        int
	MinConns        int
	MaxConnLifetime time.Duration
	// AppName shows up in pg_stat_activity, so that the connection budget can be
	// audited per service. Defaults to linochi-erp-backend.
	AppName string
	Metrics *obs.Metrics
	Tracer  trace.TracerProvider
}

// Open creates the pool, with SQL spans and query and pool metrics. pgxpool
// connects lazily, so Open succeeds while the database is down; /readyz stays
// red until it answers (NFK-02). Call Close on the pool when done.
func Open(ctx context.Context, o Options) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(o.URL)
	if err != nil {
		// pgx already masks the password in its parse errors (TestNFK14 guards that).
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = int32(o.MaxConns) //nolint:gosec // validated to 1..50 by config
	cfg.MinConns = int32(o.MinConns) //nolint:gosec // validated to 0..MaxConns by config
	cfg.MaxConnLifetime = o.MaxConnLifetime
	if o.AppName == "" {
		o.AppName = "linochi-erp-backend"
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = o.AppName

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "linochi_db_query_duration_seconds",
		Help:    "Query duration by sqlc query name.",
		Buckets: queryBuckets,
	}, []string{"query"})
	cfg.ConnConfig.Tracer = newQueryTracer(o.Tracer, duration)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := o.Metrics.Registry.Register(duration); err != nil {
		pool.Close()
		return nil, fmt.Errorf("register query metric: %w", err)
	}
	if err := o.Metrics.Registry.Register(newPoolCollector(pool)); err != nil {
		pool.Close()
		return nil, fmt.Errorf("register pool metrics: %w", err)
	}
	return pool, nil
}
