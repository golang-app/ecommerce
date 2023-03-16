package observability

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
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
