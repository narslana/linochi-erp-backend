//go:build integration

package db_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

func openWithObs(t *testing.T) (*db.Store, *obs.Metrics, *tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	t.Helper()
	dsn := migrated(t)
	metrics := obs.NewMetrics("test", "abc1234")
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	pool, err := db.Open(context.Background(), db.Options{
		URL: dsn, MaxConns: 3, MinConns: 1, MaxConnLifetime: 10 * time.Minute,
		Metrics: metrics, Tracer: tp,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return db.NewStore(pool), metrics, rec, tp
}

func families(t *testing.T, m *obs.Metrics) map[string]*dto.MetricFamily {
	t.Helper()
	fs, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*dto.MetricFamily{}
	for _, f := range fs {
		out[f.GetName()] = f
	}
	return out
}

func labels(m *dto.Metric) map[string]string {
	out := map[string]string{}
	for _, l := range m.GetLabel() {
		out[l.GetName()] = l.GetValue()
	}
	return out
}

func TestNFK04_PoolHonoursTheConfiguredLimits(t *testing.T) {
	dsn := migrated(t)
	pool, err := db.Open(context.Background(), db.Options{
		URL: dsn, MaxConns: 3, MinConns: 1, MaxConnLifetime: 10 * time.Minute,
		Metrics: obs.NewMetrics("test", "x"), Tracer: sdktrace.NewTracerProvider(),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cfg := pool.Config()
	if cfg.MaxConns != 3 || cfg.MinConns != 1 || cfg.MaxConnLifetime != 10*time.Minute {
		t.Errorf("pool config = max %d min %d life %s", cfg.MaxConns, cfg.MinConns, cfg.MaxConnLifetime)
	}
}

func TestOBS_QueryMetricsAndSpansUseTheSqlcQueryName(t *testing.T) {
	ctx := context.Background()
	store, metrics, rec, tp := openWithObs(t)

	// otelpgx only traces inside a recording span, so that no orphan root spans
	// appear. In the app every request has one.
	ctx, request := tp.Tracer("test").Start(ctx, "GET /api/v1/me")

	u, err := store.UpsertUserOnLogin(ctx, login("sub-1", "a@example.se", "A"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	got := map[string]uint64{}
	f := families(t, metrics)["linochi_db_query_duration_seconds"]
	if f == nil {
		t.Fatal("linochi_db_query_duration_seconds is missing")
	}
	for _, m := range f.GetMetric() {
		got[labels(m)["query"]] += m.GetHistogram().GetSampleCount()
	}
	for _, name := range []string{"UpsertUserOnLogin", "GetUser"} {
		if got[name] != 1 {
			t.Errorf("query %q was observed %d times, want 1 (have %v)", name, got[name], got)
		}
	}
	for q := range got {
		if strings.ContainsAny(q, " \n(") {
			t.Errorf("label value %q looks like raw SQL; it must be a bounded name", q)
		}
	}

	request.End()
	var spanNames []string
	for _, s := range rec.Ended() {
		spanNames = append(spanNames, s.Name())
		if strings.HasPrefix(s.Name(), "query ") && s.Parent().SpanID() != request.SpanContext().SpanID() {
			t.Errorf("span %q is not a child of the request span", s.Name())
		}
	}
	if !anyContains(spanNames, "query GetUser") || !anyContains(spanNames, "query UpsertUserOnLogin") {
		t.Errorf("spans %v do not carry the query names", spanNames)
	}
}

func TestOBS_FailedQueriesAreStillMeasured(t *testing.T) {
	ctx := context.Background()
	store, metrics, _, _ := openWithObs(t)
	if _, err := store.GetUser(ctx, [16]byte{}); err == nil {
		t.Fatal("expected not found")
	}
	var seen bool
	for _, m := range families(t, metrics)["linochi_db_query_duration_seconds"].GetMetric() {
		if labels(m)["query"] == "GetUser" {
			seen = true
		}
	}
	if !seen {
		t.Error("a query that returned no rows was not measured")
	}
}

func TestOBS_PoolMetricsAreExported(t *testing.T) {
	ctx := context.Background()
	store, metrics, _, _ := openWithObs(t)
	if _, err := store.UpsertUserOnLogin(ctx, login("sub-1", "a@example.se", "A")); err != nil {
		t.Fatal(err)
	}
	fs := families(t, metrics)

	conns := fs["linochi_db_pool_connections"]
	if conns == nil {
		t.Fatal("linochi_db_pool_connections is missing")
	}
	states := map[string]float64{}
	for _, m := range conns.GetMetric() {
		states[labels(m)["state"]] = m.GetGauge().GetValue()
	}
	if states["max"] != 3 {
		t.Errorf("max connections = %v, want the configured 3 (have %v)", states["max"], states)
	}
	for _, s := range []string{"idle", "in_use", "max"} {
		if _, ok := states[s]; !ok {
			t.Errorf("state %q is missing from %v", s, states)
		}
	}
	for _, name := range []string{"linochi_db_pool_acquires_total", "linochi_db_pool_acquire_wait_seconds_total"} {
		if fs[name] == nil {
			t.Errorf("%s is missing", name)
		}
	}
	if v := fs["linochi_db_pool_acquires_total"].GetMetric()[0].GetCounter().GetValue(); v < 1 {
		t.Errorf("acquires_total = %v after a query", v)
	}
}

func anyContains(list []string, s string) bool {
	for _, x := range list {
		if strings.Contains(x, s) {
			return true
		}
	}
	return false
}

func TestOBS_DBMetricNamesFollowConvention(t *testing.T) {
	store, metrics, _, _ := openWithObs(t)
	if _, err := store.GetUser(context.Background(), [16]byte{}); err == nil {
		t.Fatal("expected not found")
	}
	name := regexp.MustCompile(`^linochi_[a-z0-9]+(_[a-z0-9]+)*_(total|seconds|bytes|info|in_flight|connections)$`)
	var seen int
	for n := range families(t, metrics) {
		if !strings.HasPrefix(n, "linochi_db_") {
			continue
		}
		seen++
		if !name.MatchString(n) {
			t.Errorf("%q does not follow linochi_<area>_<thing>_<unit>", n)
		}
	}
	if seen < 6 {
		t.Errorf("expected the query and pool families, saw %d linochi_db_ families", seen)
	}
}

// Kubelet probes /readyz every few seconds. Its query has a name of its own, so
// that the catch-all label "other" keeps meaning "unnamed SQL nobody expected".
func TestOBS_ReadinessQueryHasItsOwnLabel(t *testing.T) {
	dsn := migrated(t)
	metrics := obs.NewMetrics("test", "x")
	pool, err := db.Open(context.Background(), db.Options{URL: dsn, MaxConns: 2, Metrics: metrics, Tracer: sdktrace.NewTracerProvider()})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := db.ReadinessCheck(pool)(context.Background()); err != nil {
		t.Fatal(err)
	}
	var named bool
	for _, m := range families(t, metrics)["linochi_db_query_duration_seconds"].GetMetric() {
		if labels(m)["query"] == "ReadinessSchemaVersion" {
			named = true
		}
	}
	if !named {
		t.Error("the readiness query is not measured under its own name")
	}
}
