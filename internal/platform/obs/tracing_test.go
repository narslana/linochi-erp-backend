package obs_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/obs"
)

func TestOBS_TracesAreExportedOverOTLPHTTP(t *testing.T) {
	var (
		mu   sync.Mutex
		path string
		body []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		path, body = r.URL.Path, b
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tp, err := obs.NewTracerProvider(ctx, obs.TracingConfig{Endpoint: srv.URL, Version: "v9.9.9", PodName: "erp-api-0"})
	if err != nil {
		t.Fatal(err)
	}
	_, span := tp.Tracer("test").Start(ctx, "GET /api/v1/me")
	span.End()
	if err := tp.Shutdown(ctx); err != nil { // flushes
		t.Fatalf("shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if path != "/v1/traces" {
		t.Fatalf("exported to %q, want /v1/traces", path)
	}
	var req collectortrace.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		t.Fatalf("not an OTLP request: %v", err)
	}
	rs := req.GetResourceSpans()
	if len(rs) != 1 {
		t.Fatalf("resource spans = %d", len(rs))
	}
	attrs := map[string]string{}
	for _, kv := range rs[0].GetResource().GetAttributes() {
		attrs[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	if attrs["service.name"] != "linochi-erp-backend" || attrs["service.version"] != "v9.9.9" || attrs["k8s.pod.name"] != "erp-api-0" {
		t.Errorf("resource attributes: %v", attrs)
	}
	spans := rs[0].GetScopeSpans()[0].GetSpans()
	if len(spans) != 1 || spans[0].GetName() != "GET /api/v1/me" {
		t.Errorf("spans: %v", spans)
	}
}

func TestOBS_WithoutEndpointSpansExistButNothingIsExported(t *testing.T) {
	tp, err := obs.NewTracerProvider(context.Background(), obs.TracingConfig{})
	if err != nil {
		t.Fatal(err)
	}
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	if !span.SpanContext().IsValid() {
		t.Error("spans must exist without an exporter, so that logs still carry trace_id")
	}
	span.End()
	if err := tp.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}
