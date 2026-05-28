package observability

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/bridges/otellogrus"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/trace"
)

type loggerKey struct{}

func WithLogger(ctx context.Context, logger logrus.FieldLogger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func Logger(ctx context.Context) logrus.FieldLogger {
	if logger := ctx.Value(loggerKey{}); logger != nil {
		return logger.(logrus.FieldLogger)
	}

	return logrus.New()
}

// InitLogs wires the global OpenTelemetry LoggerProvider to an OTLP/HTTP
// exporter and installs an otellogrus hook on the supplied logger so every
// logrus.Info/Error/etc. call is also emitted as an OTLP log record. When
// no OTLP endpoint is configured the function returns a noop shutdown
// closer and leaves the logger untouched — logs still go to stderr via the
// existing JSON formatter.
//
// The hook is attached to the *logrus.Logger backing the FieldLogger so
// child loggers (anything produced via WithField/WithFields) inherit it.
func InitLogs(ctx context.Context, appName string, base logrus.FieldLogger) (func(context.Context) error, error) {
	endpoint := otlpEndpointFromEnv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	if endpoint == "" {
		logrus.Info("OTel log exporter not configured (OTEL_EXPORTER_OTLP_ENDPOINT is empty); logs will not be shipped")
		return noopShutdown, nil
	}

	opts := []otlploghttp.Option{otlploghttp.WithEndpointURL(endpoint)}
	if otlpInsecureFromEnv() {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	exp, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(newResource(appName)),
	)
	global.SetLoggerProvider(provider)

	// Hook the bridge into the underlying *logrus.Logger so every entry —
	// including those routed through field-augmenting child loggers —
	// flows into OTLP as well as the JSON formatter on stderr.
	if l := rootLogger(base); l != nil {
		l.AddHook(otellogrus.NewHook(appName, otellogrus.WithLoggerProvider(provider)))
	}

	return provider.Shutdown, nil
}

// rootLogger extracts the underlying *logrus.Logger from a FieldLogger
// whether the caller passed the bare logger or a *logrus.Entry produced
// via WithField. Returns nil for any other implementation; the bridge is
// silently skipped in that case.
func rootLogger(fl logrus.FieldLogger) *logrus.Logger {
	switch v := fl.(type) {
	case *logrus.Logger:
		return v
	case *logrus.Entry:
		return v.Logger
	}
	return nil
}

func LoggerMiddleware(next http.HandlerFunc, logger logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		l := logger.WithFields(logrus.Fields{
			"trace.id": span.SpanContext().TraceID().String(),
			"span.id":  span.SpanContext().SpanID().String(),
		})

		ctx := WithLogger(r.Context(), l)
		r = r.WithContext(ctx)

		recorder := &statusRecorder{
			ResponseWriter: w,
			Status:         200,
		}

		next(recorder, r)

		l = l.WithFields(logrus.Fields{
			"status_code": recorder.Status,
			"method":      r.Method,
			"path":        r.URL.Path,
		})

		if recorder.Status >= 400 {
			l.WithFields(logrus.Fields{
				"error": http.StatusText(recorder.Status),
			}).Error("Request failed")
		} else {
			l.Info("Request succeeded")
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}
