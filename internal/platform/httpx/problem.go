// Package httpx holds the HTTP building blocks every endpoint uses: routing
// with per-route metrics and traces, RFC 9457 problem details and health checks.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// ProblemTypeBase is the prefix of the "type" member. The code that follows it
// is what n8n branches on, so codes are stable once published.
const ProblemTypeBase = "https://erp.lab.internal/problems/"

// Problem is an RFC 9457 problem details object. Domain errors implement
// ProblemError and are mapped here, not in the domain packages.
type Problem struct {
	// Code is the last part of "type", for example "version-conflict".
	Code   string
	Title  string
	Status int
	Detail string
}

// ProblemError is implemented by typed domain errors.
type ProblemError interface {
	error
	Problem() Problem
}

type wireProblem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
}

// WriteProblem writes p as application/problem+json with the request's trace_id,
// so that an error seen by a client can be found in Tempo and Loki.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	body := wireProblem{
		Type:     ProblemTypeBase + p.Code,
		Title:    p.Title,
		Status:   p.Status,
		Detail:   p.Detail,
		Instance: r.URL.Path, // never the query string: it may hold OIDC codes
	}
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		body.TraceID = sc.TraceID().String()
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(body)
}

// Error writes err as a problem. Typed errors keep their own status and text.
// Anything else becomes a generic 500: the real error is logged, never sent.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	var pe ProblemError
	if errors.As(err, &pe) {
		WriteProblem(w, r, pe.Problem())
		return
	}
	// Errors every store can return: the platform maps them once, here.
	switch {
	case errors.Is(err, db.ErrStaleVersion):
		WriteProblem(w, r, problemVersionConflict)
		return
	case errors.Is(err, db.ErrNotFound):
		WriteProblem(w, r, problemNotFound)
		return
	}
	LoggerFrom(r.Context()).ErrorContext(r.Context(), "request failed", slog.Any("error", err))
	WriteProblem(w, r, problemInternal)
}

var (
	problemInternal         = Problem{Code: "internal", Title: "Internal error", Status: http.StatusInternalServerError}
	problemVersionConflict  = Problem{Code: "version-conflict", Title: "The record was changed by someone else", Status: http.StatusConflict}
	problemNotFound         = Problem{Code: "not-found", Title: "Not found", Status: http.StatusNotFound}
	problemMethodNotAllowed = Problem{Code: "method-not-allowed", Title: "Method not allowed", Status: http.StatusMethodNotAllowed}
)

type loggerKey struct{}

// WithLogger returns a context carrying the logger.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFrom returns the logger set by the router, or the default logger.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
