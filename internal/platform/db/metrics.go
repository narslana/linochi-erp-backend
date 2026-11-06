package db

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolCollector exports pgxpool's statistics. The counters are cumulative in
// pgx, so they are exported as counters and read with rate() in Grafana.
type poolCollector struct {
	pool *pgxpool.Pool

	connections  *prometheus.Desc
	acquires     *prometheus.Desc
	acquireWait  *prometheus.Desc
	emptyAcquire *prometheus.Desc
	canceled     *prometheus.Desc
}

func newPoolCollector(pool *pgxpool.Pool) *poolCollector {
	d := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(name, help, labels, nil)
	}
	return &poolCollector{
		pool:         pool,
		connections:  d("linochi_db_pool_connections", "Pool connections by state: idle, in_use and max.", "state"),
		acquires:     d("linochi_db_pool_acquires_total", "Connections acquired from the pool."),
		acquireWait:  d("linochi_db_pool_acquire_wait_seconds_total", "Total time spent waiting to acquire a connection."),
		emptyAcquire: d("linochi_db_pool_empty_acquires_total", "Acquires that had to wait because the pool was empty. A rising rate means the pool is too small."),
		canceled:     d("linochi_db_pool_canceled_acquires_total", "Acquires cancelled while waiting for a connection."),
	}
}

// Describe implements prometheus.Collector.
func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.acquires
	ch <- c.acquireWait
	ch <- c.emptyAcquire
	ch <- c.canceled
}

// Collect implements prometheus.Collector.
func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	gauge := func(state string, v int32) {
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(v), state)
	}
	gauge("idle", s.IdleConns())
	gauge("in_use", s.AcquiredConns())
	gauge("max", s.MaxConns())
	ch <- prometheus.MustNewConstMetric(c.acquires, prometheus.CounterValue, float64(s.AcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.acquireWait, prometheus.CounterValue, s.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(c.emptyAcquire, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.canceled, prometheus.CounterValue, float64(s.CanceledAcquireCount()))
}
