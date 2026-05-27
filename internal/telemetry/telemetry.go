// Package telemetry provides OpenTelemetry tracing for HTTP calls.
// When OTEL_EXPORTER_OTLP_ENDPOINT is set, spans are exported to an OTLP collector.
// Otherwise, a no-op tracer is used.
package telemetry

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ShutdownFunc shuts down the tracer provider, flushing pending spans.
type ShutdownFunc func()

// Setup initialises the OTLP tracer provider if OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Returns a shutdown function that must be deferred by the caller.
func Setup() ShutdownFunc {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func() {}
	}

	exporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
	)
	if err != nil {
		log.Printf("telemetry: failed to create OTLP exporter: %v", err)
		return func() {}
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("eudi-conformance-evidence"),
		),
	)
	if err != nil {
		log.Printf("telemetry: failed to create resource: %v", err)
		return func() {}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("telemetry: tracer shutdown error: %v", err)
		}
	}
}

// TraceHTTP wraps an HTTP call with an OpenTelemetry span.
// The caller provides a function that executes the HTTP request and returns
// the response status code. The span records method, URL, and status.
func TraceHTTP(ctx context.Context, method, url string, fn func() (statusCode int, err error)) error {
	tracer := otel.Tracer("eudi-conformance-evidence")
	_, span := tracer.Start(ctx, fmt.Sprintf("HTTP %s %s", method, truncate(url, 120)))
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", method),
		attribute.String("http.url", url),
	)

	statusCode, err := fn()
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
	}
	if statusCode > 0 {
		span.SetAttributes(attribute.Int("http.status_code", statusCode))
	}

	return err
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
