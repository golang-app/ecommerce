package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func RuntimeMetrics(ctx context.Context, appName string) error {
	exp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return err
	}

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp)), sdkmetric.WithResource(newResource(appName)))
	otel.SetMeterProvider(meterProvider)
	err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))

	return err
}
