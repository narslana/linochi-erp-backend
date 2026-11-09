package config_test

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

const validDSN = "postgres://linochi:s3cr3t-pw@db:5432/linochi_erp?sslmode=disable"

// oidcEnv is the OIDC part of a valid environment. The secret is one a test can
// look for in output.
func oidcEnv() map[string]string {
	return map[string]string{
		"OIDC_ISSUER":        "https://auth.lab.internal",
		"OIDC_CLIENT_ID":     "linochi-erp",
		"OIDC_CLIENT_SECRET": "cl13nt-s3cret",
		"OIDC_REDIRECT_URL":  "https://erp.lab.internal/auth/callback",
	}
}

// withOIDC is m plus a valid OIDC part, for the tests that are about something else.
func withOIDC(m map[string]string) map[string]string {
	out := oidcEnv()
	maps.Copy(out, m)
	return out
}

var wantOIDC = config.OIDC{
	Issuer: "https://auth.lab.internal", ClientID: "linochi-erp", ClientSecret: "cl13nt-s3cret",
	RedirectURL: "https://erp.lab.internal/auth/callback", AdminGroups: []string{"admins", "linochi_erp_users"},
}

func TestNFK02_ConfigLoadsFromEnvOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.Config
	}{
		{
			name: "defaults",
			env:  withOIDC(map[string]string{"DATABASE_URL": validDSN}),
			want: config.Config{
				OIDC:            wantOIDC,
				HTTPAddr:        ":8080",
				InternalAddr:    ":9090",
				DatabaseURL:     validDSN,
				LogLevel:        "info",
				ShutdownTimeout: 20 * time.Second,
				ShutdownDelay:   5 * time.Second,
				DBMaxConns:      5,
				DBMinConns:      0,
				DBMaxConnLife:   30 * time.Minute,
			},
		},
		{
			name: "overrides",
			env: withOIDC(map[string]string{
				"DATABASE_URL":                validDSN,
				"HTTP_ADDR":                   "127.0.0.1:9000",
				"INTERNAL_ADDR":               "127.0.0.1:9001",
				"LOG_LEVEL":                   "debug",
				"SHUTDOWN_TIMEOUT":            "5s",
				"SHUTDOWN_DELAY":              "0s",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://alloy.alloy.svc.cluster.local:4318",
				"POD_NAME":                    "erp-api-7d9f",
				"DB_MAX_CONNS":                "8",
				"DB_MIN_CONNS":                "2",
				"DB_MAX_CONN_LIFETIME":        "10m",
			}),
			want: config.Config{
				OIDC:            wantOIDC,
				HTTPAddr:        "127.0.0.1:9000",
				InternalAddr:    "127.0.0.1:9001",
				DatabaseURL:     validDSN,
				LogLevel:        "debug",
				ShutdownTimeout: 5 * time.Second,
				ShutdownDelay:   0,
				OTLPEndpoint:    "http://alloy.alloy.svc.cluster.local:4318",
				PodName:         "erp-api-7d9f",
				DBMaxConns:      8,
				DBMinConns:      2,
				DBMaxConnLife:   10 * time.Minute,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(env(tt.env))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNFK02_ConfigRejectsMissingOrInvalid(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		field string
	}{
		{"missing database url", map[string]string{}, "DATABASE_URL"},
		{"database url not a url", map[string]string{"DATABASE_URL": "::nope"}, "DATABASE_URL"},
		{"database url wrong scheme", map[string]string{"DATABASE_URL": "mysql://u:p@h/db"}, "DATABASE_URL"},
		{"bad shutdown timeout", map[string]string{"DATABASE_URL": validDSN, "SHUTDOWN_TIMEOUT": "soon"}, "SHUTDOWN_TIMEOUT"},
		{"non-positive shutdown timeout", map[string]string{"DATABASE_URL": validDSN, "SHUTDOWN_TIMEOUT": "0s"}, "SHUTDOWN_TIMEOUT"},
		{"bad shutdown delay", map[string]string{"DATABASE_URL": validDSN, "SHUTDOWN_DELAY": "soon"}, "SHUTDOWN_DELAY"},
		{"negative shutdown delay", map[string]string{"DATABASE_URL": validDSN, "SHUTDOWN_DELAY": "-1s"}, "SHUTDOWN_DELAY"},
		{"otlp endpoint not a url", map[string]string{"DATABASE_URL": validDSN, "OTEL_EXPORTER_OTLP_ENDPOINT": "alloy:4318"}, "OTEL_EXPORTER_OTLP_ENDPOINT"},
		{"db max conns not a number", map[string]string{"DATABASE_URL": validDSN, "DB_MAX_CONNS": "many"}, "DB_MAX_CONNS"},
		{"db max conns zero", map[string]string{"DATABASE_URL": validDSN, "DB_MAX_CONNS": "0"}, "DB_MAX_CONNS"},
		{"db max conns above the budget", map[string]string{"DATABASE_URL": validDSN, "DB_MAX_CONNS": "51"}, "DB_MAX_CONNS"},
		{"db min conns negative", map[string]string{"DATABASE_URL": validDSN, "DB_MIN_CONNS": "-1"}, "DB_MIN_CONNS"},
		{"db min conns above max", map[string]string{"DATABASE_URL": validDSN, "DB_MAX_CONNS": "3", "DB_MIN_CONNS": "4"}, "DB_MIN_CONNS"},
		{"db conn lifetime invalid", map[string]string{"DATABASE_URL": validDSN, "DB_MAX_CONN_LIFETIME": "long"}, "DB_MAX_CONN_LIFETIME"},
		{"bad log level", map[string]string{"DATABASE_URL": validDSN, "LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"bad http addr", map[string]string{"DATABASE_URL": validDSN, "HTTP_ADDR": "no-port"}, "HTTP_ADDR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(env(tt.env))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ce *config.Error
			if !errors.As(err, &ce) {
				t.Fatalf("error %T is not *config.Error: %v", err, err)
			}
			if ce.Field != tt.field {
				t.Errorf("Field = %q, want %q", ce.Field, tt.field)
			}
			if strings.Contains(err.Error(), "s3cr3t-pw") {
				t.Errorf("error leaks the password: %v", err)
			}
		})
	}
}

func TestNFK14_ConfigStringMasksDatabasePassword(t *testing.T) {
	cfg, err := config.Load(env(withOIDC(map[string]string{"DATABASE_URL": validDSN})))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for name, out := range map[string]string{
		"String": cfg.String(),
		"%v":     fmt.Sprintf("%v", cfg),
		"%+v":    fmt.Sprintf("%+v", cfg),
		"%s":     fmt.Sprintf("%s", cfg), //nolint:staticcheck // exercising the %s verb on purpose
	} {
		if strings.Contains(out, "s3cr3t-pw") {
			t.Errorf("%s leaks the password: %s", name, out)
		}
		if !strings.Contains(out, "db:5432") {
			t.Errorf("%s should still show the host: %s", name, out)
		}
	}
}

// `api migrate` runs as the database owner, with its own URL. The app's
// DATABASE_URL (the restricted role) is not needed by the migration Job.
func TestNFK28_LoadMigrateReadsOnlyTheMigrateURL(t *testing.T) {
	const ownerDSN = "postgres://linochi_erp_owner:0wn3r-pw@db:5432/linochi_erp?sslmode=disable"
	cfg, err := config.LoadMigrate(env(map[string]string{"MIGRATE_DATABASE_URL": ownerDSN}))
	if err != nil {
		t.Fatalf("LoadMigrate: %v", err)
	}
	if cfg.DatabaseURL != ownerDSN {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if out := cfg.String(); strings.Contains(out, "0wn3r-pw") || !strings.Contains(out, "db:5432") {
		t.Errorf("String must mask the password and keep the host: %s", out)
	}
}

func TestNFK28_LoadMigrateRejectsMissingOrInvalid(t *testing.T) {
	for name, m := range map[string]map[string]string{
		"missing":      {},
		"only the app": {"DATABASE_URL": validDSN}, // running migrations as the app role is a mistake
		"wrong scheme": {"MIGRATE_DATABASE_URL": "mysql://u:p@h/db"},
		"not a url":    {"MIGRATE_DATABASE_URL": "::nope"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := config.LoadMigrate(env(m))
			var ce *config.Error
			if !errors.As(err, &ce) || ce.Field != "MIGRATE_DATABASE_URL" {
				t.Fatalf("err = %v, want a config.Error for MIGRATE_DATABASE_URL", err)
			}
		})
	}
}

func TestNFK10_ConfigTakesTheAdminGroupsFromTheEnvironment(t *testing.T) {
	for name, tc := range map[string]struct {
		value string
		want  []string
	}{
		"one":                 {"admins", []string{"admins"}},
		"several, with space": {" admins , ops,linochi_erp_users ", []string{"admins", "ops", "linochi_erp_users"}},
		"repeated":            {"admins,admins", []string{"admins"}},
		"stray commas":        {",admins,,", []string{"admins"}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := config.Load(env(withOIDC(map[string]string{"DATABASE_URL": validDSN, "AUTH_ADMIN_GROUPS": tc.value})))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.OIDC.AdminGroups, tc.want) {
				t.Errorf("AdminGroups = %q, want %q", cfg.OIDC.AdminGroups, tc.want)
			}
		})
	}
}

func TestNFK10_ConfigRejectsMissingOrUnsafeOIDCSettings(t *testing.T) {
	with := func(k, v string) map[string]string {
		m := withOIDC(map[string]string{"DATABASE_URL": validDSN})
		if v == "" {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name  string
		env   map[string]string
		field string
	}{
		{"no issuer", with("OIDC_ISSUER", ""), "OIDC_ISSUER"},
		{"no client id", with("OIDC_CLIENT_ID", ""), "OIDC_CLIENT_ID"},
		{"no client secret", with("OIDC_CLIENT_SECRET", ""), "OIDC_CLIENT_SECRET"},
		{"no redirect url", with("OIDC_REDIRECT_URL", ""), "OIDC_REDIRECT_URL"},
		{"issuer is not a URL", with("OIDC_ISSUER", "auth.lab.internal"), "OIDC_ISSUER"},
		{"issuer over plain http", with("OIDC_ISSUER", "http://auth.lab.internal"), "OIDC_ISSUER"},
		{"issuer with a query", with("OIDC_ISSUER", "https://auth.lab.internal?x=1"), "OIDC_ISSUER"},
		{"issuer with a fragment", with("OIDC_ISSUER", "https://auth.lab.internal#x"), "OIDC_ISSUER"},
		{"issuer with credentials", with("OIDC_ISSUER", "https://user:pw@auth.lab.internal"), "OIDC_ISSUER"},
		{"redirect over plain http", with("OIDC_REDIRECT_URL", "http://erp.lab.internal/auth/callback"), "OIDC_REDIRECT_URL"},
		{"redirect to another path", with("OIDC_REDIRECT_URL", "https://erp.lab.internal/callback"), "OIDC_REDIRECT_URL"},
		{"redirect with a query", with("OIDC_REDIRECT_URL", "https://erp.lab.internal/auth/callback?x=1"), "OIDC_REDIRECT_URL"},
		{"redirect with a fragment", with("OIDC_REDIRECT_URL", "https://erp.lab.internal/auth/callback#x"), "OIDC_REDIRECT_URL"},
		{"only blank admin groups", func() map[string]string { m := with("", ""); m["AUTH_ADMIN_GROUPS"] = " , "; return m }(), "AUTH_ADMIN_GROUPS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Load(env(tt.env))
			var ce *config.Error
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v, want a *config.Error", err)
			}
			if ce.Field != tt.field {
				t.Errorf("Field = %q, want %q", ce.Field, tt.field)
			}
			if strings.Contains(err.Error(), "cl13nt-s3cret") || strings.Contains(err.Error(), "pw@") {
				t.Errorf("the error leaks a secret: %v", err)
			}
		})
	}
}

// http is for a provider on this machine (cmd/devoidc) and for nothing else.
func TestNFK10_ConfigAllowsPlainHTTPOnLoopbackOnly(t *testing.T) {
	for _, host := range []string{"localhost:9096", "127.0.0.1:9096", "[::1]:9096"} {
		m := withOIDC(map[string]string{
			"DATABASE_URL": validDSN, "OIDC_ISSUER": "http://" + host, "OIDC_REDIRECT_URL": "http://" + host + "/auth/callback",
		})
		if _, err := config.Load(env(m)); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	for _, host := range []string{"192.168.1.10:9096", "auth.lab.internal", "localhost.evil.example", "0.0.0.0:9096"} {
		m := withOIDC(map[string]string{"DATABASE_URL": validDSN, "OIDC_ISSUER": "http://" + host})
		if _, err := config.Load(env(m)); err == nil {
			t.Errorf("http://%s was accepted as an issuer", host)
		}
	}
}

func TestNFK14_ConfigStringMasksTheClientSecret(t *testing.T) {
	cfg, err := config.Load(env(withOIDC(map[string]string{"DATABASE_URL": validDSN})))
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{
		"String": cfg.String(), "%v": fmt.Sprintf("%v", cfg), "%+v": fmt.Sprintf("%+v", cfg),
		"OIDC %+v": fmt.Sprintf("%+v", cfg.OIDC), "OIDC %#v": fmt.Sprintf("%#v", cfg.OIDC), "OIDC %v": fmt.Sprintf("%v", cfg.OIDC),
	} {
		if strings.Contains(out, "cl13nt-s3cret") {
			t.Errorf("%s leaks the client secret: %s", name, out)
		}
	}
	if !strings.Contains(cfg.String(), "https://auth.lab.internal") || !strings.Contains(cfg.String(), "linochi-erp") {
		t.Errorf("the issuer and client id are worth logging: %s", cfg.String())
	}
}

// The operator command has to work when the login is what is broken, so it must
// not need the login's settings (api sessions revoke-user is how access is
// removed in an emergency).
func TestNFK10_LoadOperatorNeedsTheDatabaseAndNothingElse(t *testing.T) {
	cfg, err := config.LoadOperator(env(map[string]string{"DATABASE_URL": validDSN, "LOG_LEVEL": "debug"}))
	if err != nil {
		t.Fatalf("LoadOperator: %v", err)
	}
	if cfg.DatabaseURL != validDSN || cfg.SlogLevel().String() != "DEBUG" {
		t.Errorf("cfg = %+v", cfg)
	}
	if strings.Contains(cfg.String(), "s3cr3t-pw") {
		t.Errorf("String leaks the password: %s", cfg)
	}

	for name, m := range map[string]map[string]string{
		"missing":            {},
		"only the owner url": {"MIGRATE_DATABASE_URL": validDSN},
		"wrong scheme":       {"DATABASE_URL": "mysql://u:p@h/db"},
		"bad log level":      {"DATABASE_URL": validDSN, "LOG_LEVEL": "loud"},
	} {
		_, err := config.LoadOperator(env(m))
		var ce *config.Error
		if !errors.As(err, &ce) {
			t.Errorf("%s: err = %v, want a *config.Error", name, err)
		}
	}
}
