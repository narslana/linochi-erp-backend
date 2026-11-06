package httpx_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/httpx"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	return rec
}

func TestNFK02_HealthzIsAlwaysOK(t *testing.T) {
	h := &httpx.Health{}
	h.AddCheck("db", func(context.Context) error { return errors.New("down") })
	h.SetDraining()
	// Liveness must not depend on anything: a database outage must not make
	// Kubernetes restart healthy pods in a loop.
	if rec := get(h.Healthz(), "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestNFK02_ReadyzIs503WhileDraining(t *testing.T) {
	h := &httpx.Health{}
	if rec := get(h.Readyz(), "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("before draining: %d", rec.Code)
	}
	h.SetDraining()
	rec := get(h.Readyz(), "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("while draining: %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["status"] != "draining" {
		t.Errorf("body = %s (%v)", rec.Body.String(), err)
	}
}

func TestNFK02_ReadyzReportsFailingCheckWithTimeout(t *testing.T) {
	h := &httpx.Health{CheckTimeout: 50 * time.Millisecond}
	h.AddCheck("fine", func(context.Context) error { return nil })
	h.AddCheck("broken", func(context.Context) error { return errors.New("dial tcp 10.0.0.5: refused") })
	h.AddCheck("hangs", func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })

	start := time.Now()
	rec := get(h.Readyz(), "/readyz")
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("/readyz hung for %s: a stuck check must not block it", took)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"fine": "ok", "broken": "failed", "hangs": "timeout"}
	for k, v := range want {
		if body.Checks[k] != v {
			t.Errorf("check %q = %q, want %q", k, body.Checks[k], v)
		}
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("error detail leaked: %s", rec.Body.String())
	}
}
