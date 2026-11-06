package db

import (
	"context"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
)

// queryBuckets are finer than the HTTP ones: most queries take well under 10 ms.
var queryBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5}

// queryTracer gives every query an OpenTelemetry span (otelpgx) and records its
// duration, labelled with the sqlc query name. pgx accepts one tracer, so this
// wraps the otelpgx one and keeps its batch, prepare and acquire tracing.
type queryTracer struct {
	*otelpgx.Tracer
	duration *prometheus.HistogramVec
}

func newQueryTracer(tp trace.TracerProvider, duration *prometheus.HistogramVec) *queryTracer {
	return &queryTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tp),
			// "query GetUser" and "prepare GetUser": the prefix tells pgx's two spans apart.
			otelpgx.WithQuerySpanNamePrefix(),
			otelpgx.WithSpanNameFunc(QueryName),
		),
		duration: duration,
	}
}

type queryStart struct {
	at   time.Time
	name string
}

type queryStartKey struct{}

// TraceQueryStart implements pgx.QueryTracer.
func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	return context.WithValue(ctx, queryStartKey{}, queryStart{at: time.Now(), name: QueryName(data.SQL)})
}

// TraceQueryEnd implements pgx.QueryTracer. It also runs for failed queries and
// for ones that return no rows, so slow failures are visible too.
func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if s, ok := ctx.Value(queryStartKey{}).(queryStart); ok {
		t.duration.WithLabelValues(s.name).Observe(time.Since(s.at).Seconds())
	}
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}
