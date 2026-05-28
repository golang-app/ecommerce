package mailer_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/sirupsen/logrus"
)

func TestLogMailerSendDoesNotError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	var buf bytes.Buffer
	logger.SetOutput(&buf)

	m := &mailer.LogMailer{Logger: logger, From: "no-reply@example.com"}
	err := m.Send(context.Background(), mailer.Message{
		To:       "alice@example.com",
		Subject:  "hello",
		TextBody: "world",
	})
	if err != nil {
		t.Fatalf("LogMailer.Send: %v", err)
	}
	if !strings.Contains(buf.String(), "alice@example.com") {
		t.Fatalf("log output missing recipient: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("log output missing subject: %q", buf.String())
	}
}

func TestLogMailerRejectsEmptyTo(t *testing.T) {
	m := &mailer.LogMailer{Logger: logrus.New()}
	err := m.Send(context.Background(), mailer.Message{Subject: "x", TextBody: "y"})
	if err == nil {
		t.Fatalf("expected error for empty To")
	}
}

func TestNewSelectsLogMailerWhenHostEmpty(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(new(bytes.Buffer))
	m := mailer.New(mailer.Config{Host: "", From: "x@example.com"}, logger)
	if _, ok := m.(*mailer.LogMailer); !ok {
		t.Fatalf("expected LogMailer, got %T", m)
	}
}

func TestNewSelectsSMTPMailerWhenHostSet(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(new(bytes.Buffer))
	m := mailer.New(mailer.Config{Host: "mailhog:1025", From: "x@example.com"}, logger)
	if _, ok := m.(*mailer.SMTPMailer); !ok {
		t.Fatalf("expected SMTPMailer, got %T", m)
	}
}

func TestSMTPMailerRejectsEmptyMessage(t *testing.T) {
	// Construction-only check; we don't actually dial. With both bodies
	// blank, Send must short-circuit BEFORE attempting any network IO.
	m := &mailer.SMTPMailer{Host: "127.0.0.1:0", From: "x@example.com"}
	err := m.Send(context.Background(), mailer.Message{To: "y@example.com"})
	if err == nil {
		t.Fatalf("expected ErrEmptyMessage")
	}
}
