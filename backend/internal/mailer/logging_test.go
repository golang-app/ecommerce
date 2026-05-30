package mailer

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func newCapturedLogger() (*logrus.Logger, *bytes.Buffer) {
	logger := logrus.New()
	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.TextFormatter{DisableColors: true, DisableTimestamp: true})
	return logger, buf
}

func TestLoggingMailerLogsSuccess(t *testing.T) {
	logger, buf := newCapturedLogger()
	inner := &fakeMailer{}
	l := NewLogging(inner, logger)

	err := l.Send(context.Background(), Message{
		To:      "alice@example.com",
		Subject: "Receipt",
		Kind:    KindOrderConfirmation,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner should have been called once, got %d", inner.calls)
	}
	out := buf.String()
	if !strings.Contains(out, "mailer.send") {
		t.Fatalf("expected mailer.send log entry, got %q", out)
	}
	if !strings.Contains(out, "alice@example.com") {
		t.Fatalf("expected to= field, got %q", out)
	}
	if !strings.Contains(out, KindOrderConfirmation) {
		t.Fatalf("expected kind field, got %q", out)
	}
	if strings.Contains(out, "mailer.send failed") {
		t.Fatalf("did NOT expect failure entry, got %q", out)
	}
}

func TestLoggingMailerLogsFailureAndReturnsError(t *testing.T) {
	logger, buf := newCapturedLogger()
	boom := errors.New("smtp dial: timeout")
	inner := &fakeMailer{errs: []error{boom}}
	l := NewLogging(inner, logger)

	err := l.Send(context.Background(), Message{To: "alice@example.com", Subject: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom propagated, got %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "mailer.send failed") {
		t.Fatalf("expected failure log entry, got %q", out)
	}
	if !strings.Contains(out, "smtp dial: timeout") {
		t.Fatalf("expected inner error in log, got %q", out)
	}
}

func TestLoggingMailerNilLoggerIsPassthrough(t *testing.T) {
	inner := &fakeMailer{}
	l := NewLogging(inner, nil)

	if err := l.Send(context.Background(), Message{To: "a@b"}); err != nil {
		t.Fatalf("nil logger should still proxy successfully: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("inner should have been called once, got %d", inner.calls)
	}
}
