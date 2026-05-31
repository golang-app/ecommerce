package mailer

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
)

// emitter is the metric sink the decorator depends on. The default
// production wiring is observability.EmailsSentInc; tests inject their
// own recorder.
type emitter func(ctx context.Context, kind, outcome string)

// MetricsMailer wraps an inner Mailer and increments
// gocommerce_emails_sent_total{kind, outcome} EXACTLY ONCE per Message —
// regardless of how many retries the inner stack performed. That is why
// MetricsMailer must sit OUTSIDE RetryingMailer in the composition: the
// counter records the final outcome of an end-to-end Send, not per
// attempt. (See cmd/web/main.go for the wiring + the inline rationale.)
//
// The leaf SMTPMailer / LogMailer used to increment this counter
// themselves; that responsibility was moved here so retries do not
// double-count and so the metric is owned by a single, easily-disabled
// decorator.
type MetricsMailer struct {
	inner Mailer
	emit  emitter
}

// NewMetrics constructs a MetricsMailer that writes to the application
// emails_sent_total counter via observability.EmailsSentInc. A nil inner
// mailer is a programming error; we don't guard against it because
// composition happens once at startup and a nil inner would crash on the
// very first publish.
func NewMetrics(inner Mailer) *MetricsMailer {
	return &MetricsMailer{
		inner: inner,
		emit:  observability.EmailsSentInc,
	}
}

// newMetricsWithEmitter is the test-only constructor that lets the unit
// test capture metric writes without touching the global OTEL meter.
func newMetricsWithEmitter(inner Mailer, emit emitter) *MetricsMailer {
	return &MetricsMailer{inner: inner, emit: emit}
}

// Send delegates to the inner mailer and records exactly one
// emails_sent_total observation tagged by Kind and outcome
// ("success"/"failure"). It returns the inner error verbatim.
func (m *MetricsMailer) Send(ctx context.Context, msg Message) error {
	err := m.inner.Send(ctx, msg)
	outcome := "success"
	if err != nil {
		outcome = "failure"
	}
	if m.emit != nil {
		m.emit(ctx, kindOrDefault(msg.Kind), outcome)
	}
	return err
}
