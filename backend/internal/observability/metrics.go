package observability

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// RuntimeMetrics installs an OTLP/HTTP metric pipeline and starts the
// runtime metrics collector. The returned shutdown closer flushes the
// periodic reader. When no OTLP endpoint is configured we still start the
// runtime collector and return a noop closer so callers do not need to
// branch on the no-export case.
func RuntimeMetrics(ctx context.Context, appName string) (func(context.Context) error, error) {
	endpoint := otlpEndpointFromEnv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	if endpoint == "" {
		logrus.Info("OTel metrics exporter not configured (OTEL_EXPORTER_OTLP_ENDPOINT is empty); metrics will not be shipped")
		if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
			return noopShutdown, err
		}
		return noopShutdown, nil
	}

	opts := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(endpoint)}
	if otlpInsecureFromEnv() {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exp, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)),
		sdkmetric.WithResource(newResource(appName)),
	)
	otel.SetMeterProvider(meterProvider)

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		return meterProvider.Shutdown, err
	}

	return meterProvider.Shutdown, nil
}
