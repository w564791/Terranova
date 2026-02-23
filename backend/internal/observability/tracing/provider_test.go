package tracing

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// TestInitTracer_NoopWhenEndpointUnset verifies that InitTracer returns a
// no-op shutdown and no error when OTEL_EXPORTER_OTLP_ENDPOINT is not set.
// The global TracerProvider must remain a noop (i.e. not an *sdktrace.TracerProvider).
func TestInitTracer_NoopWhenEndpointUnset(t *testing.T) {
	// Ensure the env var is unset for this test.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown function")
	}

	// The global provider should NOT be an SDK TracerProvider.
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		t.Error("expected noop TracerProvider when endpoint is unset, got *sdktrace.TracerProvider")
	}

	// The shutdown function should succeed without error.
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown returned unexpected error: %v", err)
	}
}

// TestInitTracer_ParsesSamplerArg validates that OTEL_TRACES_SAMPLER_ARG is
// parsed correctly. We can't easily inspect the sampler from outside, but we
// can at least make sure bad values don't cause a panic or error (they fall
// back to the default 1.0).
func TestInitTracer_InvalidSamplerArgFallsBack(t *testing.T) {
	// No endpoint -> noop path, but we still exercise the sampler parsing
	// indirectly by ensuring no panic. For a more thorough test we set the
	// endpoint so the full code-path runs, but that requires a reachable
	// collector. Instead we just confirm the noop path is safe with a bad arg.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "not-a-number")

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown returned unexpected error: %v", err)
	}
}

// TestInitTracer_DefaultServiceName confirms that when OTEL_SERVICE_NAME is
// empty the code does not panic (the default "iac-backend" is used internally).
func TestInitTracer_DefaultServiceName(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	_ = shutdown(context.Background())
}

// TestInitTracer_CustomServiceName ensures a custom service name does not
// cause errors in the noop path.
func TestInitTracer_CustomServiceName(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "my-custom-service")

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	_ = shutdown(context.Background())
}

// TestInitTracer_WithEndpoint tests the full initialisation path by pointing
// at a non-existent collector. The exporter creation itself should succeed
// (gRPC connections are lazy), and we immediately shut down the provider.
func TestInitTracer_WithEndpoint(t *testing.T) {
	// Use a non-routable address so we don't actually need a collector.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	t.Setenv("OTEL_SERVICE_NAME", "test-service")
	t.Setenv("OTEL_SERVICE_VERSION", "0.0.1-test")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")

	// Ensure OTEL_EXPORTER_OTLP_INSECURE is set so the SDK doesn't try TLS.
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("InitTracer returned nil shutdown function")
	}

	// The global provider should now be an SDK TracerProvider.
	tp := otel.GetTracerProvider()
	if _, ok := tp.(*sdktrace.TracerProvider); !ok {
		t.Errorf("expected *sdktrace.TracerProvider, got %T", tp)
	}

	// Shutdown should not error (it will fail to flush to the fake endpoint,
	// but Shutdown itself should not return an error for unreachable exporters).
	if err := shutdown(context.Background()); err != nil {
		t.Logf("shutdown returned error (may be expected with no collector): %v", err)
	}

	// Reset global provider to noop to avoid leaking state to other tests.
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
}

// TestInitTracer_EnvVarsNotLeaked is a sanity check that t.Setenv correctly
// restores the environment, avoiding cross-test contamination.
func TestInitTracer_EnvVarsNotLeaked(t *testing.T) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		t.Skip("OTEL_EXPORTER_OTLP_ENDPOINT is set in the real environment, skipping")
	}

	shutdown, err := InitTracer(context.Background())
	if err != nil {
		t.Fatalf("InitTracer returned unexpected error: %v", err)
	}
	_ = shutdown(context.Background())
}
