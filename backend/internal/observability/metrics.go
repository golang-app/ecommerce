package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/sdk/metric"
)

func RuntimeMetrics(ctx context.Context, appName string) error {
	exp, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return err
	}

	meterProvider := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(exp)), metric.WithResource(newResource(appName)))
	global.SetMeterProvider(meterProvider)
	err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))

	return err
}
