package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

// RouterOptions configures a Router.
type RouterOptions struct {
	// Server labels metrics: "public" or "internal".
	Server  string
	Logger  *slog.Logger
	Metrics *obs.Metrics
	Tracer  trace.TracerProvider
}

// Router is an http.ServeMux that traces, measures and logs every route.
//
// Instrumentation is attached per route when the route is registered, so the
// span name and the "route" label are the pattern ("GET /api/v1/items/{id}")
// and never the raw path. Requests that match nothing share the single route
// "unmatched", so the number of label values cannot be inflated by a scanner.
// The instrumentation cannot sit outside the mux: it would not know the
// pattern, because the mux only sets it on its own copy of the request.
type Router struct {
	mux       *http.ServeMux
	opts      RouterOptions
	unmatched http.Handler
}

// NewRouter creates a Router.
func NewRouter(o RouterOptions) *Router {
	r := &Router{mux: http.NewServeMux(), opts: o}
	r.unmatched = r.instrument("unmatched", http.HandlerFunc(r.serveUnmatched))
	return r
}

// Handle registers h for pattern, with instrumentation.
func (r *Router) Handle(pattern string, h http.Handler) {
	r.mux.Handle(pattern, r.instrument(pattern, h))
}

type fallbackKey struct{}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h, pattern := r.mux.Handler(req)
	if pattern == "" {
		// Not found, method not allowed or a path-cleaning redirect. h is the
		// mux's own answer; serveUnmatched turns it into a problem.
		r.unmatched.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), fallbackKey{}, h)))
		return
	}
	r.mux.ServeHTTP(w, req)
}

func (r *Router) serveUnmatched(w http.ResponseWriter, req *http.Request) {
	h, _ := req.Context().Value(fallbackKey{}).(http.Handler)
	probe := &probeWriter{header: http.Header{}}
	h.ServeHTTP(probe, req)
	switch probe.status {
	case http.StatusNotFound:
		WriteProblem(w, req, problemNotFound)
	case http.StatusMethodNotAllowed:
		if allow := probe.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		WriteProblem(w, req, problemMethodNotAllowed)
	default:
		h.ServeHTTP(w, req) // a redirect: let it through as the mux wrote it
	}
}

// probeWriter records what the mux would answer, without sending it.
type probeWriter struct {
	header http.Header
	status int
}

func (p *probeWriter) Header() http.Header         { return p.header }
func (p *probeWriter) WriteHeader(status int)      { p.status = status }
func (p *probeWriter) Write(b []byte) (int, error) { return len(b), nil }

// instrument wraps h: trace span (outermost, so it sees the final status), then
// metrics and access log, then panic recovery.
func (r *Router) instrument(route string, h http.Handler) http.Handler {
	observed := r.observe(route, h)
	return otelhttp.NewHandler(observed, route,
		otelhttp.WithTracerProvider(r.opts.Tracer),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		// otelhttp would name the span from r.Pattern, which it does not see
		// (the mux sets it on its own copy). The route is known here.
		otelhttp.WithSpanNameFormatter(func(operation string, _ *http.Request) string { return operation }),
	)
}

func (r *Router) observe(route string, next http.Handler) http.Handler {
	o := r.opts
	level := slog.LevelInfo
	if o.Server == "internal" {
		level = slog.LevelDebug // probes and scrapes every few seconds
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		ctx := WithLogger(req.Context(), o.Logger)
		req = req.WithContext(ctx)
		sw := &statusWriter{ResponseWriter: w}

		o.Metrics.HTTPInFlight.WithLabelValues(o.Server).Inc()
		defer o.Metrics.HTTPInFlight.WithLabelValues(o.Server).Dec()

		func() {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler { //nolint:errorlint // recover() returns the sentinel itself
					panic(v)
				}
				o.Logger.ErrorContext(ctx, "panic in handler",
					slog.Any("panic", v), slog.String("stack", string(debug.Stack())))
				if !sw.wrote {
					WriteProblem(sw, req, problemInternal)
				}
			}()
			next.ServeHTTP(sw, req)
		}()

		elapsed := time.Since(start)
		status := sw.statusOrOK()
		method := metricMethod(req.Method)
		o.Metrics.HTTPRequests.WithLabelValues(o.Server, method, route, strconv.Itoa(status)).Inc()
		o.Metrics.HTTPDuration.WithLabelValues(o.Server, method, route).Observe(elapsed.Seconds())

		// req.URL.Path only: the query string can hold OIDC codes and tokens.
		o.Logger.Log(ctx, level, "request",
			slog.String("method", req.Method),
			slog.String("route", route),
			slog.String("path", req.URL.Path),
			slog.Int("status", status),
			slog.Int64("bytes", sw.bytes),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
		)
	})
}

// metricMethod keeps unknown methods from creating new metric series.
func metricMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	}
	return "other"
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err //nolint:wrapcheck // io.Writer contract
}

func (s *statusWriter) statusOrOK() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

// Unwrap lets http.ResponseController reach the real writer (flush, deadlines).
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
