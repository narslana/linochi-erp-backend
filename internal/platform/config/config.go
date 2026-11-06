// Package config reads the application's configuration from the environment.
//
// Nothing is read from files (NFK-02, NFK-14): in the cluster the environment
// is filled from a ConfigMap and a Sealed Secret, locally from .env.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"
)

// Config is the validated configuration of the API process.
type Config struct {
	// HTTPAddr is where the public API listens.
	HTTPAddr string
	// InternalAddr is where /metrics and, later, API-key routes listen. It has
	// no ingress route.
	InternalAddr string
	// DatabaseURL is the PostgreSQL connection string. It contains a secret.
	DatabaseURL string
	// LogLevel is a slog level name: debug, info, warn or error.
	LogLevel string
	// ShutdownTimeout is how long in-flight requests get to finish on SIGTERM.
	ShutdownTimeout time.Duration
	// ShutdownDelay is how long the process keeps serving after /readyz has
	// turned 503, so that Traefik and the endpoints controller stop sending
	// traffic before the listeners close (NFK-02).
	ShutdownDelay time.Duration
	// OTLPEndpoint is the OTLP/HTTP base URL of Alloy. Empty means traces are
	// created (so logs carry trace_id) but not exported.
	OTLPEndpoint string
	// PodName identifies the replica in traces. Optional.
	PodName string
	// DBMaxConns is the pool size per pod. The CNPG cluster has 100 connections
	// for all services, so the budget is 5 per pod (DECISIONS.md 2026-11-06).
	DBMaxConns int
	// DBMinConns is how many connections the pool keeps open when idle.
	DBMinConns int
	// DBMaxConnLife is how long a connection is reused before it is replaced.
	DBMaxConnLife time.Duration
}

// maxDBConnsCeiling stops a typo from eating the whole cluster's connections.
const maxDBConnsCeiling = 50

// Error says which environment variable is wrong. It never contains the value
// of a variable, since values may be secrets.
type Error struct {
	Field  string
	Reason string
}

func (e *Error) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Field, e.Reason)
}

// Load reads and validates the configuration. getenv is os.Getenv in
// production and a map lookup in tests.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:        orDefault(getenv("HTTP_ADDR"), ":8080"),
		InternalAddr:    orDefault(getenv("INTERNAL_ADDR"), ":9090"),
		DatabaseURL:     getenv("DATABASE_URL"),
		LogLevel:        orDefault(getenv("LOG_LEVEL"), "info"),
		ShutdownTimeout: 20 * time.Second,
		ShutdownDelay:   5 * time.Second,
		OTLPEndpoint:    getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		PodName:         getenv("POD_NAME"),
		DBMaxConns:      5,
		DBMinConns:      0,
		DBMaxConnLife:   30 * time.Minute,
	}

	if err := validateAddr("HTTP_ADDR", cfg.HTTPAddr); err != nil {
		return Config{}, err
	}
	if err := validateAddr("INTERNAL_ADDR", cfg.InternalAddr); err != nil {
		return Config{}, err
	}
	if err := validateDatabaseURL(cfg.DatabaseURL); err != nil {
		return Config{}, err
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return Config{}, &Error{Field: "LOG_LEVEL", Reason: "must be debug, info, warn or error"}
	}

	if cfg.OTLPEndpoint != "" {
		u, err := url.Parse(cfg.OTLPEndpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, &Error{Field: "OTEL_EXPORTER_OTLP_ENDPOINT", Reason: "must be an http(s) URL such as http://alloy.alloy.svc.cluster.local:4318"}
		}
	}

	if v := getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxDBConnsCeiling {
			return Config{}, &Error{Field: "DB_MAX_CONNS", Reason: fmt.Sprintf("must be a whole number from 1 to %d", maxDBConnsCeiling)}
		}
		cfg.DBMaxConns = n
	}
	if v := getenv("DB_MIN_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, &Error{Field: "DB_MIN_CONNS", Reason: "must be a whole number, zero or more"}
		}
		cfg.DBMinConns = n
	}
	if cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, &Error{Field: "DB_MIN_CONNS", Reason: "must not be larger than DB_MAX_CONNS"}
	}
	if v := getenv("DB_MAX_CONN_LIFETIME"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, &Error{Field: "DB_MAX_CONN_LIFETIME", Reason: "must be a positive duration such as 30m"}
		}
		cfg.DBMaxConnLife = d
	}

	if v := getenv("SHUTDOWN_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, &Error{Field: "SHUTDOWN_DELAY", Reason: "must be a duration such as 5s"}
		}
		if d < 0 {
			return Config{}, &Error{Field: "SHUTDOWN_DELAY", Reason: "must not be negative"}
		}
		cfg.ShutdownDelay = d
	}

	if v := getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, &Error{Field: "SHUTDOWN_TIMEOUT", Reason: "must be a duration such as 20s"}
		}
		if d <= 0 {
			return Config{}, &Error{Field: "SHUTDOWN_TIMEOUT", Reason: "must be greater than zero"}
		}
		cfg.ShutdownTimeout = d
	}

	return cfg, nil
}

// SlogLevel returns LogLevel as a slog.Level. Load has already validated it.
func (c Config) SlogLevel() slog.Level {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	return level
}

// String masks the password in DatabaseURL (NFK-14), so that a Config can be
// logged or printed without leaking secrets.
func (c Config) String() string {
	return fmt.Sprintf(
		"Config{HTTPAddr:%s InternalAddr:%s DatabaseURL:%s LogLevel:%s ShutdownTimeout:%s ShutdownDelay:%s OTLPEndpoint:%s PodName:%s DBMaxConns:%d DBMinConns:%d DBMaxConnLife:%s}",
		c.HTTPAddr, c.InternalAddr, maskDSN(c.DatabaseURL), c.LogLevel, c.ShutdownTimeout, c.ShutdownDelay, c.OTLPEndpoint, c.PodName, c.DBMaxConns, c.DBMinConns, c.DBMaxConnLife,
	)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func validateAddr(field, addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return &Error{Field: field, Reason: "must be host:port, for example :8080"}
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	if raw == "" {
		return &Error{Field: "DATABASE_URL", Reason: "is required"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &Error{Field: "DATABASE_URL", Reason: "is not a valid URL"}
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return &Error{Field: "DATABASE_URL", Reason: "must start with postgres:// or postgresql://"}
	}
	return nil
}

func maskDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	return u.Redacted()
}
