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
	"slices"
	"strconv"
	"strings"
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
	// OIDC is the login through Authelia (NFK-10).
	OIDC OIDC
}

// Secret is a value that must not reach a log or an error message, whichever
// way a struct holding it is printed (NFK-14).
type Secret string

// String masks the value.
func (Secret) String() string { return "[masked]" }

// GoString masks the value for %#v.
func (Secret) GoString() string { return `"[masked]"` }

// LogValue masks the value for slog.
func (Secret) LogValue() slog.Value { return slog.StringValue("[masked]") }

// OIDC is how the app logs people in through Authelia (NFK-10).
type OIDC struct {
	// Issuer is Authelia's URL, https://auth.lab.internal. The discovery document
	// must claim exactly this.
	Issuer   string
	ClientID string
	// ClientSecret authenticates the app to Authelia. Only its hash is in the
	// Authelia configuration; this is the clear text, from a Sealed Secret.
	ClientSecret Secret
	// RedirectURL is the callback registered in Authelia, matched exactly there.
	RedirectURL string
	// AdminGroups are the Authelia groups that give the admin role (DECISIONS.md
	// 2026-11-05). The mapping from group to role is configuration, so a read
	// role can be added later without a code change.
	AdminGroups []string
}

// callbackPath is the route the provider sends the browser back to.
const callbackPath = "/auth/callback"

// defaultAdminGroups are the two groups of the requirement (NFK-10).
var defaultAdminGroups = []string{"admins", "linochi_erp_users"}

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
	if err := validateDatabaseURL("DATABASE_URL", cfg.DatabaseURL); err != nil {
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

	oidc, err := loadOIDC(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.OIDC = oidc

	return cfg, nil
}

// loadOIDC reads the login settings. All four are required to serve: an app
// that starts without a way to log in only looks healthy.
func loadOIDC(getenv func(string) string) (OIDC, error) {
	o := OIDC{
		Issuer: getenv("OIDC_ISSUER"), ClientID: getenv("OIDC_CLIENT_ID"),
		ClientSecret: Secret(getenv("OIDC_CLIENT_SECRET")), RedirectURL: getenv("OIDC_REDIRECT_URL"),
	}
	if err := validateProviderURL("OIDC_ISSUER", o.Issuer, ""); err != nil {
		return OIDC{}, err
	}
	if o.ClientID == "" {
		return OIDC{}, &Error{Field: "OIDC_CLIENT_ID", Reason: "is required"}
	}
	if o.ClientSecret == "" {
		return OIDC{}, &Error{Field: "OIDC_CLIENT_SECRET", Reason: "is required"}
	}
	if err := validateProviderURL("OIDC_REDIRECT_URL", o.RedirectURL, callbackPath); err != nil {
		return OIDC{}, err
	}

	o.AdminGroups = slices.Clone(defaultAdminGroups)
	if v := getenv("AUTH_ADMIN_GROUPS"); v != "" {
		o.AdminGroups = nil
		for g := range strings.SplitSeq(v, ",") {
			if g = strings.TrimSpace(g); g != "" && !slices.Contains(o.AdminGroups, g) {
				o.AdminGroups = append(o.AdminGroups, g)
			}
		}
		if len(o.AdminGroups) == 0 {
			return OIDC{}, &Error{Field: "AUTH_ADMIN_GROUPS", Reason: "must name at least one group, for example admins,linochi_erp_users"}
		}
	}
	return o, nil
}

// validateProviderURL checks a URL that points at the login: https, or http on
// this machine for a development provider and nowhere else. If path is not
// empty the URL must have exactly that path. The value is never put in the
// error, since a URL can carry credentials.
func validateProviderURL(field, raw, path string) error {
	if raw == "" {
		return &Error{Field: field, Reason: "is required"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return &Error{Field: field, Reason: "must be an https URL"}
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopback(u.Hostname())) {
		return &Error{Field: field, Reason: "must be an https URL (http is allowed only for localhost, 127.0.0.1 and [::1])"}
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return &Error{Field: field, Reason: "must have no credentials, query or fragment"}
	}
	if path != "" && u.Path != path {
		return &Error{Field: field, Reason: "must end in " + path + ", the route the app serves"}
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// OperatorConfig is the configuration of the operator commands (`api sessions`):
// the app role's database and the log level, and nothing else. They are run by
// hand, often because something is wrong, so they must not need the login's
// settings or anything else the server needs to start.
type OperatorConfig struct {
	// DatabaseURL is the app role's connection string. It contains a secret.
	DatabaseURL string
	// LogLevel is a slog level name.
	LogLevel string
}

// LoadOperator reads the configuration of the operator commands. It is the app
// role's DATABASE_URL, never the owner's.
func LoadOperator(getenv func(string) string) (OperatorConfig, error) {
	cfg := OperatorConfig{DatabaseURL: getenv("DATABASE_URL"), LogLevel: orDefault(getenv("LOG_LEVEL"), "info")}
	if err := validateDatabaseURL("DATABASE_URL", cfg.DatabaseURL); err != nil {
		return OperatorConfig{}, err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		return OperatorConfig{}, &Error{Field: "LOG_LEVEL", Reason: "must be debug, info, warn or error"}
	}
	return cfg, nil
}

// SlogLevel returns LogLevel as a slog.Level. LoadOperator has validated it.
func (c OperatorConfig) SlogLevel() slog.Level {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.LogLevel))
	return level
}

// String masks the password, like Config.String (NFK-14).
func (c OperatorConfig) String() string {
	return fmt.Sprintf("OperatorConfig{DatabaseURL:%s LogLevel:%s}", maskDSN(c.DatabaseURL), c.LogLevel)
}

// MigrateConfig is the configuration of `api migrate`. Migrations run as the
// database owner, a different role from the restricted one the app runs as, so
// the Job has a URL of its own and does not need the app's DATABASE_URL.
type MigrateConfig struct {
	// DatabaseURL is the owner's connection string, from MIGRATE_DATABASE_URL.
	// It contains a secret.
	DatabaseURL string
}

// LoadMigrate reads the configuration of `api migrate`. There is no fallback to
// DATABASE_URL: the app role cannot change the schema, and a migration that
// quietly used it would only fail later and confusingly.
func LoadMigrate(getenv func(string) string) (MigrateConfig, error) {
	cfg := MigrateConfig{DatabaseURL: getenv("MIGRATE_DATABASE_URL")}
	if err := validateDatabaseURL("MIGRATE_DATABASE_URL", cfg.DatabaseURL); err != nil {
		return MigrateConfig{}, err
	}
	return cfg, nil
}

// String masks the password, like Config.String (NFK-14).
func (c MigrateConfig) String() string {
	return fmt.Sprintf("MigrateConfig{DatabaseURL:%s}", maskDSN(c.DatabaseURL))
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
		"Config{HTTPAddr:%s InternalAddr:%s DatabaseURL:%s LogLevel:%s ShutdownTimeout:%s ShutdownDelay:%s OTLPEndpoint:%s PodName:%s DBMaxConns:%d DBMinConns:%d DBMaxConnLife:%s OIDC:{Issuer:%s ClientID:%s ClientSecret:%s RedirectURL:%s AdminGroups:%v}}",
		c.HTTPAddr, c.InternalAddr, maskDSN(c.DatabaseURL), c.LogLevel, c.ShutdownTimeout, c.ShutdownDelay, c.OTLPEndpoint, c.PodName, c.DBMaxConns, c.DBMinConns, c.DBMaxConnLife,
		c.OIDC.Issuer, c.OIDC.ClientID, c.OIDC.ClientSecret, c.OIDC.RedirectURL, c.OIDC.AdminGroups,
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

func validateDatabaseURL(field, raw string) error {
	if raw == "" {
		return &Error{Field: field, Reason: "is required"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &Error{Field: field, Reason: "is not a valid URL"}
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return &Error{Field: field, Reason: "must start with postgres:// or postgresql://"}
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
