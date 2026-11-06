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
}

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
		"Config{HTTPAddr:%s InternalAddr:%s DatabaseURL:%s LogLevel:%s ShutdownTimeout:%s ShutdownDelay:%s OTLPEndpoint:%s PodName:%s}",
		c.HTTPAddr, c.InternalAddr, maskDSN(c.DatabaseURL), c.LogLevel, c.ShutdownTimeout, c.ShutdownDelay, c.OTLPEndpoint, c.PodName,
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
