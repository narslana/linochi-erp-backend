package obs

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the Prometheus registry. It is a registry of our own rather
// than the global default, so that nothing registers itself behind our back
// (AGENTS.md: no globals) and tests can start from scratch.
type Metrics struct {
	Registry *prometheus.Registry

	// HTTP metrics, labelled by server ("public" or "internal") and route
	// pattern, never by raw path.
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	HTTPInFlight *prometheus.GaugeVec
}

// DurationBuckets has bounds exactly on 0.2 s and 0.5 s, so that the NFK-03
// targets (p95 < 200 ms, p99 < 500 ms) can be read and alerted on.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 1, 2.5, 5}

// NewMetrics creates the registry with Go runtime, process and build info
// collectors and the HTTP metrics.
func NewMetrics(version, commit string) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		HTTPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "linochi_http_requests_total",
			Help: "HTTP requests by server, method, route pattern and status.",
		}, []string{"server", "method", "route", "status"}),
		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "linochi_http_request_duration_seconds",
			Help:    "HTTP request duration by server, method and route pattern.",
			Buckets: DurationBuckets,
		}, []string{"server", "method", "route"}),
		HTTPInFlight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "linochi_http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}, []string{"server"}),
	}
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "linochi_build_info",
		Help: "Build information. Always 1.",
	}, []string{"version", "commit"})
	buildInfo.WithLabelValues(version, commit).Set(1)

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		buildInfo, m.HTTPRequests, m.HTTPDuration, m.HTTPInFlight,
	)
	return m
}

// Handler serves the registry in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Registry: m.Registry})
}
