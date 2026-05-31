package mailer

import (
	"context"

	"github.com/sirupsen/logrus"
)

// LoggingMailer wraps an inner Mailer with structured-log breadcrumbs:
// one "mailer.send" entry on entry, one "mailer.send failed" entry on
// error. It deliberately does NOT log the body — payloads can contain
// password-reset tokens or order PII that should not land in the logs
// twice (the leaf LogMailer already redacts to a 200-char preview).
//
// LoggingMailer is the outermost decorator in the standard composition:
// it sees the FINAL outcome of the retry loop, so a transient failure
// followed by a successful retry produces ONE info entry, not three.
type LoggingMailer struct {
	inner  Mailer
	logger logrus.FieldLogger
}

// NewLogging constructs a LoggingMailer. A nil logger is permitted (the
// decorator becomes a passthrough) so unit tests that don't care about
// the log output can skip the fake-logger boilerplate.
func NewLogging(inner Mailer, logger logrus.FieldLogger) *LoggingMailer {
	return &LoggingMailer{inner: inner, logger: logger}
}

// Send logs the outbound attempt, delegates to the inner mailer, and
// logs the failure (if any). The returned error is the inner error
// verbatim so upstream callers can still type-check it.
func (l *LoggingMailer) Send(ctx context.Context, msg Message) error {
	fields := logrus.Fields{
		"to":      msg.To,
		"subject": msg.Subject,
		"kind":    kindOrDefault(msg.Kind),
	}
	if l.logger != nil {
		l.logger.WithFields(fields).Info("mailer.send")
	}
	err := l.inner.Send(ctx, msg)
	if err != nil && l.logger != nil {
		l.logger.WithFields(fields).WithError(err).Error("mailer.send failed")
	}
	return err
}
