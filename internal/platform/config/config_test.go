package config_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/config"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

const validDSN = "postgres://linochi:s3cr3t-pw@db:5432/linochi_erp?sslmode=disable"

func TestNFK02_ConfigLoadsFromEnvOnly(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.Config
	}{
		{
			name: "defaults",
			env:  map[string]string{"DATABASE_URL": validDSN},
			want: config.Config{
				HTTPAddr:        ":8080",
				InternalAddr:    ":9090",
				DatabaseURL:     validDSN,
				LogLevel:        "info",
				ShutdownTimeout: 20 * time.Second,
				ShutdownDelay:   5 * time.Second,
			},
		},
		{
			name: "overrides",
			env: map[string]string{
				"DATABASE_URL":                validDSN,
				"HTTP_ADDR":                   "127.0.0.1:9000",
				"INTERNAL_ADDR":               "127.0.0.1:9001",
				"LOG_LEVEL":                   "debug",
				"SHUTDOWN_TIMEOUT":            "5s",
				"SHUTDOWN_DELAY":              "0s",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://alloy.alloy.svc.cluster.local:4318",
				"POD_NAME":                    "erp-api-7d9f",
			},
			want: config.Config{
				HTTPAddr:        "127.0.0.1:9000",
				InternalAddr:    "127.0.0.1:9001",
				DatabaseURL:     validDSN,
				LogLevel:        "debug",
				ShutdownTimeout: 5 * time.Second,
				ShutdownDelay:   0,
				OTLPEndpoint:    "http://alloy.alloy.svc.cluster.local:4318",
				PodName:         "erp-api-7d9f",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(env(tt.env))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got != tt.want {
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
	cfg, err := config.Load(env(map[string]string{"DATABASE_URL": validDSN}))
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
