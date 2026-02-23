// Package tracing initialises the OpenTelemetry TracerProvider.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is set the package creates an OTLP/gRPC
// exporter with a BatchSpanProcessor, configures a TraceIDRatioBased sampler,
// and registers the provider globally.  When the variable is unset the global
// provider remains a noop, which means all tracing calls are zero-cost.
package tracing

import (
	"context"
	"log"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracer bootstraps the OpenTelemetry tracing pipeline.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, tracing is disabled and a no-op
// shutdown function is returned without error.
//
// The returned shutdown function flushes pending spans and should be called
// on application exit:
//
//	shutdown, err := tracing.InitTracer(ctx)
//	if err != nil { ... }
//	defer shutdown(ctx)
func InitTracer(ctx context.Context) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		log.Println("[tracing] OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled")
		return func(context.Context) error { return nil }, nil
	}

	// --- exporter -----------------------------------------------------------
	// otlptracegrpc.New reads OTEL_EXPORTER_OTLP_ENDPOINT (and other standard
	// env vars) automatically, so we don't pass options for the endpoint.
	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	// --- resource -----------------------------------------------------------
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "iac-backend"
	}

	resAttrs := []resource.Option{
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	}
	if v := os.Getenv("OTEL_SERVICE_VERSION"); v != "" {
		resAttrs = append(resAttrs, resource.WithAttributes(semconv.ServiceVersion(v)))
	}

	svcRes, err := resource.New(ctx, resAttrs...)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(resource.Default(), svcRes)
	if err != nil {
		return nil, err
	}

	// --- sampler ------------------------------------------------------------
	ratio := 1.0
	if arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); arg != "" {
		if parsed, parseErr := strconv.ParseFloat(arg, 64); parseErr == nil {
			ratio = parsed
		} else {
			log.Printf("[tracing] invalid OTEL_TRACES_SAMPLER_ARG %q, using default 1.0", arg)
		}
	}

	// --- provider -----------------------------------------------------------
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(ratio)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Printf("[tracing] tracing enabled, exporting to %s", endpoint)

	return tp.Shutdown, nil
}
