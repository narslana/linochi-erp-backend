package obs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"log/slog"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

func logLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("log line is not JSON: %v\n%s", err, buf.String())
	}
	return m
}

func TestOBS_EveryRequestLogLineHasTraceID(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.NewLogger(&buf, slog.LevelInfo)
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()
	logger.InfoContext(ctx, "inside a request")

	m := logLine(t, &buf)
	if got, want := m["trace_id"], span.SpanContext().TraceID().String(); got != want {
		t.Errorf("trace_id = %v, want %v", got, want)
	}
	if got, want := m["span_id"], span.SpanContext().SpanID().String(); got != want {
		t.Errorf("span_id = %v, want %v", got, want)
	}
}

func TestOBS_LogLineOutsideARequestHasNoFakeTraceID(t *testing.T) {
	var buf bytes.Buffer
	obs.NewLogger(&buf, slog.LevelInfo).InfoContext(context.Background(), "startup")
	if _, ok := logLine(t, &buf)["trace_id"]; ok {
		t.Error("a line without a span must not carry a zero trace_id")
	}
}

func TestOBS_PersonalDataIsMaskedInLogs(t *testing.T) {
	tests := []struct {
		name string
		attr slog.Attr
		leak string
	}{
		{"name", slog.String("name", "Anna Svensson"), "Anna"},
		{"email", slog.String("email", "anna@example.se"), "anna@"},
		{"phone", slog.String("phone", "070-123 45 67"), "070"},
		{"address", slog.String("address", "Storgatan 1"), "Storgatan"},
		{"suffix _name", slog.String("customer_name", "Butik AB"), "Butik"},
		{"suffix _email", slog.String("contact_email", "x@y.se"), "x@y.se"},
		{"upper case", slog.String("Email", "Z@z.se"), "Z@z.se"},
		{"in a group", slog.Group("customer", slog.String("email", "g@g.se")), "g@g.se"},
		{"Personal value under any key", slog.Any("note", obs.Personal("Kalle Kula")), "Kalle"},
		{"password", slog.String("password", "hunter2"), "hunter2"},
		{"token", slog.String("token", "tok-123"), "tok-123"},
		{"authorization", slog.String("authorization", "Bearer abc"), "Bearer abc"},
		{"cookie", slog.String("cookie", "__Host-s=abc"), "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			obs.NewLogger(&buf, slog.LevelInfo).Info("event", tt.attr)
			out := buf.String()
			if strings.Contains(out, tt.leak) {
				t.Errorf("clear text %q leaked: %s", tt.leak, out)
			}
			if !strings.Contains(out, "[masked]") {
				t.Errorf("expected [masked] in %s", out)
			}
		})
	}
}

func TestOBS_OrdinaryFieldsAreNotMasked(t *testing.T) {
	var buf bytes.Buffer
	obs.NewLogger(&buf, slog.LevelInfo).Info("event",
		slog.String("route", "GET /api/v1/me"), slog.String("hostname", "pi-a"), slog.Int("status", 200))
	m := logLine(t, &buf)
	if m["route"] != "GET /api/v1/me" || m["hostname"] != "pi-a" || m["status"] != float64(200) {
		t.Errorf("ordinary fields were altered: %v", m)
	}
}
