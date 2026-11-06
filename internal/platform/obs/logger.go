// Package obs holds logging, metrics and tracing set-up.
package obs

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Masked replaces personal data and secrets in logs.
const Masked = "[masked]"

// Personal marks a value as personal data (name, e-mail, phone, address). It is
// masked in logs whatever key it is logged under (AGENTS.md: persondata
// maskeras). Use it when the key alone would not reveal what the value is.
type Personal string

// LogValue implements slog.LogValuer.
func (Personal) LogValue() slog.Value { return slog.StringValue(Masked) }

// NewLogger returns the application logger: JSON to w, with trace_id and
// span_id when the context carries a span, and personal data masked.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: mask})
	return slog.New(traceHandler{h})
}

// sensitiveKeys are masked by exact (lower-case) key.
var sensitiveKeys = map[string]struct{}{
	"name": {}, "email": {}, "e-mail": {}, "phone": {}, "address": {},
	"password": {}, "secret": {}, "token": {}, "authorization": {}, "cookie": {},
}

// sensitiveSuffixes are masked by key suffix, so that customer_name,
// contact_email and so on are covered.
var sensitiveSuffixes = []string{"_name", "_email", "_phone", "_address", "_password", "_secret", "_token"}

func mask(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		return a // its members are visited on their own
	}
	key := strings.ToLower(a.Key)
	if _, ok := sensitiveKeys[key]; ok {
		return slog.String(a.Key, Masked)
	}
	for _, s := range sensitiveSuffixes {
		if strings.HasSuffix(key, s) {
			return slog.String(a.Key, Masked)
		}
	}
	return a
}

// traceHandler adds trace_id and span_id from the context. Lines logged
// outside a span get neither, rather than an all-zero id that would look real.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r) //nolint:wrapcheck // a handler must return the inner handler's error unchanged
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}
