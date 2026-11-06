package obs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ServiceName is the service.name in traces.
const ServiceName = "linochi-erp-backend"

// TracingConfig configures the tracer provider.
type TracingConfig struct {
	// Endpoint is the OTLP/HTTP base URL (Alloy on :4318). Empty means spans
	// are created but not exported.
	Endpoint string
	Version  string
	PodName  string
}

// NewTracerProvider returns a provider that always creates spans, so that
// trace_id is in the logs even when nothing is exported (local development).
// With an Endpoint it also exports over OTLP/HTTP. Call Shutdown on exit to
// flush. Sampling is parent-based and records everything: the volume is low.
func NewTracerProvider(ctx context.Context, cfg TracingConfig) (*sdktrace.TracerProvider, error) {
	attrs := []attribute.KeyValue{attribute.String("service.name", ServiceName)}
	if cfg.Version != "" {
		attrs = append(attrs, attribute.String("service.version", cfg.Version))
	}
	if cfg.PodName != "" {
		attrs = append(attrs, attribute.String("k8s.pod.name", cfg.PodName), attribute.String("service.instance.id", cfg.PodName))
	}
	res, err := resource.New(ctx, resource.WithTelemetrySDK(), resource.WithAttributes(attrs...))
	if err != nil {
		return nil, fmt.Errorf("trace resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	if cfg.Endpoint != "" {
		// WithEndpointURL uses the URL path as is ("/" when there is none), so
		// the signal path is added here: Endpoint is a base URL, as in the
		// OTLP exporter specification.
		u, err := url.Parse(cfg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("otlp endpoint: %w", err)
		}
		u.Path = strings.TrimSuffix(u.Path, "/") + "/v1/traces"
		exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(u.String()))
		if err != nil {
			return nil, fmt.Errorf("otlp trace exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	return sdktrace.NewTracerProvider(opts...), nil
}
