package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

type env struct {
	router  *httpx.Router
	logs    *bytes.Buffer
	metrics *obs.Metrics
	spans   *tracetest.SpanRecorder
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{logs: &bytes.Buffer{}, metrics: obs.NewMetrics("test", "abc1234"), spans: tracetest.NewSpanRecorder()}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(e.spans))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	e.router = httpx.NewRouter(httpx.RouterOptions{
		Server:  "public",
		Logger:  obs.NewLogger(e.logs, slog.LevelInfo),
		Metrics: e.metrics,
		Tracer:  tp,
	})
	return e
}

func (e *env) do(method, target string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *env) families(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()
	fs, err := e.metrics.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*dto.MetricFamily{}
	for _, f := range fs {
		out[f.GetName()] = f
	}
	return out
}

func labelsOf(m *dto.Metric) map[string]string {
	out := map[string]string{}
	for _, l := range m.GetLabel() {
		out[l.GetName()] = l.GetValue()
	}
	return out
}

type conflictError struct{}

func (conflictError) Error() string { return "version mismatch on item 7" }
func (conflictError) Problem() httpx.Problem {
	return httpx.Problem{Code: "version-conflict", Title: "Posten har ändrats av någon annan", Status: http.StatusConflict}
}

func TestAPI_ErrorsAreRFC9457ProblemDetails(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /api/v1/ok", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	e.router.Handle("GET /api/v1/conflict", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, conflictError{})
	}))
	e.router.Handle("GET /api/v1/untyped", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, errors.New("pq: password authentication failed for user linochi"))
	}))

	tests := []struct {
		name, method, path string
		status             int
		code               string
		allow              string
	}{
		{"not found", "GET", "/finns-inte", 404, "not-found", ""},
		{"method not allowed", "POST", "/api/v1/ok", 405, "method-not-allowed", "GET"},
		{"typed domain error", "GET", "/api/v1/conflict", 409, "version-conflict", ""},
		{"untyped error is a generic 500", "GET", "/api/v1/untyped", 500, "internal", ""},
	}
	traceID := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(tt.method, tt.path)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
			if tt.allow != "" && !strings.Contains(rec.Header().Get("Allow"), tt.allow) {
				t.Errorf("Allow = %q, want it to contain %q", rec.Header().Get("Allow"), tt.allow)
			}
			var p map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if int(p["status"].(float64)) != tt.status {
				t.Errorf("status in body = %v", p["status"])
			}
			if p["type"] != "https://erp.lab.internal/problems/"+tt.code {
				t.Errorf("type = %v", p["type"])
			}
			if s, _ := p["title"].(string); s == "" {
				t.Error("title is empty")
			}
			if s, _ := p["trace_id"].(string); !traceID.MatchString(s) {
				t.Errorf("trace_id = %v", p["trace_id"])
			}
			if strings.Contains(rec.Body.String(), "password authentication") || strings.Contains(rec.Body.String(), "item 7") {
				t.Errorf("internal error text leaked: %s", rec.Body.String())
			}
		})
	}
}

func TestAPI_PanicBecomes500ProblemWithoutInternals(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /boom", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom: db password is hunter2") }))

	rec := e.do("GET", "/boom")
	if rec.Code != 500 || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status=%d content-type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("panic text leaked to the client: %s", rec.Body.String())
	}
	if !strings.Contains(e.logs.String(), "boom") || !strings.Contains(e.logs.String(), "stack") {
		t.Errorf("panic and stack must be logged: %s", e.logs.String())
	}
	// And the request is still counted, as a 500.
	var counted bool
	for _, m := range e.families(t)["linochi_http_requests_total"].GetMetric() {
		if l := labelsOf(m); l["route"] == "GET /boom" && l["status"] == "500" {
			counted = true
		}
	}
	if !counted {
		t.Error("the panicking request was not counted as status 500")
	}
}

func TestNFK03_DurationHistogramPerRouteHasSLOBuckets(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /api/v1/items/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	e.do("GET", "/api/v1/items/42")
	e.do("GET", "/api/v1/items/43")

	f := e.families(t)["linochi_http_request_duration_seconds"]
	if f == nil || len(f.GetMetric()) != 1 {
		t.Fatalf("want exactly one series for two ids, got %v", f)
	}
	m := f.GetMetric()[0]
	if l := labelsOf(m); l["route"] != "GET /api/v1/items/{id}" || l["server"] != "public" || l["method"] != "GET" {
		t.Errorf("labels = %v", l)
	}
	if m.GetHistogram().GetSampleCount() != 2 {
		t.Errorf("sample count = %d", m.GetHistogram().GetSampleCount())
	}
	have := map[float64]bool{}
	for _, b := range m.GetHistogram().GetBucket() {
		have[b.GetUpperBound()] = true
	}
	if !have[0.2] || !have[0.5] {
		t.Errorf("buckets must include 0.2 and 0.5 (NFK-03), have %v", have)
	}
}

func TestOBS_UnmatchedPathsShareOneRouteLabel(t *testing.T) {
	e := newEnv(t)
	for i := range 100 {
		e.do("GET", fmt.Sprintf("/scan/%d/x%d", i, i*7))
	}
	e.do("BREW", "/coffee")

	routes := map[string]bool{}
	methods := map[string]bool{}
	for _, m := range e.families(t)["linochi_http_requests_total"].GetMetric() {
		l := labelsOf(m)
		routes[l["route"]] = true
		methods[l["method"]] = true
	}
	if len(routes) != 1 || !routes["unmatched"] {
		t.Errorf("routes = %v, want only \"unmatched\"", routes)
	}
	if !methods["GET"] || !methods["other"] || len(methods) != 2 {
		t.Errorf("methods = %v, want GET and other (unknown methods must not create series)", methods)
	}
}

func TestOBS_MetricNamesFollowConvention(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /x", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	e.do("GET", "/x")

	name := regexp.MustCompile(`^linochi_[a-z0-9]+(_[a-z0-9]+)*_(total|seconds|bytes|info|in_flight)$`)
	var seen int
	for n := range e.families(t) {
		if !strings.HasPrefix(n, "linochi_") {
			continue
		}
		seen++
		if !name.MatchString(n) {
			t.Errorf("%q does not follow linochi_<område>_<sak>_<enhet>", n)
		}
	}
	if seen < 4 {
		t.Errorf("expected the http and build metrics, saw %d linochi_ families", seen)
	}
}

func TestOBS_AccessLogOmitsQueryStringAndHasTraceID(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /auth/callback", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	e.do("GET", "/auth/callback?code=hemlig-kod&state=abc")

	out := e.logs.String()
	if strings.Contains(out, "hemlig-kod") || strings.Contains(out, "state=abc") {
		t.Fatalf("query string was logged: %s", out)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &line); err != nil {
		t.Fatalf("want exactly one JSON access-log line: %v\n%s", err, out)
	}
	if line["path"] != "/auth/callback" || line["route"] != "GET /auth/callback" || line["status"] != float64(200) || line["method"] != "GET" {
		t.Errorf("access log fields: %v", line)
	}
	spans := e.spans.Ended()
	if len(spans) != 1 || line["trace_id"] != spans[0].SpanContext().TraceID().String() {
		t.Errorf("log trace_id %v does not match the request span", line["trace_id"])
	}
}

func TestOBS_RequestSpanIsNamedAfterRoutePattern(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /api/v1/items/{id}", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	e.do("GET", "/api/v1/items/42")
	e.do("GET", "/nope/123")

	var names []string
	for _, s := range e.spans.Ended() {
		names = append(names, s.Name())
	}
	if len(names) != 2 || names[0] != "GET /api/v1/items/{id}" || names[1] != "unmatched" {
		t.Errorf("span names = %v", names)
	}
}

func TestOBS_IncomingTraceparentIsContinued(t *testing.T) {
	e := newEnv(t)
	e.router.Handle("GET /x", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	e.do("GET", "/x", "traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	spans := e.spans.Ended()
	if len(spans) != 1 || spans[0].SpanContext().TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("the request span did not join the caller's trace: %v", spans)
	}
}
