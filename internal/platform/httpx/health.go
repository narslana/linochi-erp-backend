package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Health serves liveness and readiness.
//
// Liveness (/healthz) depends on nothing, so that a database outage does not
// make Kubernetes restart healthy pods in a loop. Readiness (/readyz) turns 503
// when a check fails and, first of all, when the process starts draining on
// SIGTERM, so that traffic stops before the listeners close (NFK-02).
type Health struct {
	// CheckTimeout bounds each check. Zero means 2 seconds.
	CheckTimeout time.Duration

	draining atomic.Bool
	mu       sync.RWMutex
	checks   []namedCheck
}

type namedCheck struct {
	name string
	fn   func(context.Context) error
}

// AddCheck registers a readiness check. B3 adds the database.
func (h *Health) AddCheck(name string, fn func(context.Context) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, namedCheck{name, fn})
}

// SetDraining makes /readyz answer 503 from now on.
func (h *Health) SetDraining() { h.draining.Store(true) }

// Healthz is the liveness handler.
func (h *Health) Healthz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

type readyBody struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Readyz is the readiness handler. Failure details are logged by the checks'
// owners, never sent: the body says only ok, failed or timeout.
func (h *Health) Readyz() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.draining.Load() {
			writeJSON(w, http.StatusServiceUnavailable, readyBody{Status: "draining"})
			return
		}
		h.mu.RLock()
		checks := append([]namedCheck(nil), h.checks...)
		h.mu.RUnlock()

		timeout := h.CheckTimeout
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		results := make([]string, len(checks))
		var wg sync.WaitGroup
		for i, c := range checks {
			wg.Add(1)
			go func(ctx context.Context) {
				defer wg.Done()
				results[i] = runCheck(ctx, timeout, c.fn)
			}(r.Context())
		}
		wg.Wait()

		body := readyBody{Status: "ok", Checks: map[string]string{}}
		for i, c := range checks {
			body.Checks[c.name] = results[i]
			if results[i] != "ok" {
				body.Status = "unavailable"
			}
		}
		code := http.StatusOK
		if body.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, body)
	})
}

// runCheck gives up after timeout even if fn ignores its context, so that a
// stuck dependency cannot make /readyz itself hang.
func runCheck(parent context.Context, timeout time.Duration, fn func(context.Context) error) string {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()
	select {
	case err := <-done:
		if err == nil {
			return "ok"
		}
		return "failed"
	case <-ctx.Done():
		return "timeout"
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
