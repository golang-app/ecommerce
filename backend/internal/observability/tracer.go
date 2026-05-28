package observability

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type TracerOptions struct {
	Env     string
	AppName string
}

// noopShutdown is returned when no exporter is configured so callers can
// always invoke the returned closer without nil-checking.
func noopShutdown(context.Context) error { return nil }

// otlpEndpointFromEnv returns the OTLP endpoint URL configured via the
// standard OTEL environment variables. An empty string means "exporter
// disabled" — InitTracer / RuntimeMetrics skip exporter setup in that case
// so the application keeps running with no traces/metrics shipped.
func otlpEndpointFromEnv(signalEnv string) string {
	if v := strings.TrimSpace(os.Getenv(signalEnv)); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
}

// otlpInsecureFromEnv reads OTEL_EXPORTER_OTLP_INSECURE (the SDK does not
// honour this verbatim across all transports; we interpret it ourselves and
// translate to the per-exporter WithInsecure() option). Values 1/true/yes
// are treated as true.
func otlpInsecureFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")))
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	switch v {
	case "yes", "y", "on":
		return true
	}
	return false
}

func InitTracer(ctx context.Context, settings TracerOptions) (func(context.Context) error, trace.Tracer, error) {
	endpoint := otlpEndpointFromEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	if endpoint == "" {
		logrus.Info("OTel trace exporter not configured (OTEL_EXPORTER_OTLP_ENDPOINT is empty); traces will not be shipped")
		// Still install a TracerProvider with the configured resource so
		// the rest of the app can call otel.Tracer(...) without nil panics.
		otel.SetTracerProvider(
			sdktrace.NewTracerProvider(
				sdktrace.WithSampler(sdktrace.AlwaysSample()),
				sdktrace.WithResource(newResource(settings.AppName)),
			),
		)
		return noopShutdown, otel.Tracer(""), nil
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(endpoint)}
	if otlpInsecureFromEnv() {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	client := otlptracehttp.NewClient(opts...)
	exp, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	// WithBatcher is non-blocking — the batcher tolerates a collector
	// being momentarily unreachable. The application boots even when the
	// dev observability stack is down.
	otel.SetTracerProvider(
		sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(exp),
			sdktrace.WithResource(newResource(settings.AppName)),
		),
	)

	return exp.Shutdown, otel.Tracer(""), nil
}

func newResource(appName string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(appName),
	)
}

// Tracer returns the tracer associated with the given instrumentation name.
// A thin wrapper over otel.Tracer kept here so other packages can depend on
// the observability module instead of importing go.opentelemetry.io/otel
// directly — this keeps the OTel SDK surface localised to one place and lets
// us swap providers without touching every call site.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
