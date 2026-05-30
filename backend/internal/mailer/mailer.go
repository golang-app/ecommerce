// Package mailer is the outbound-email abstraction: the rest of the app talks
// to a Mailer interface and never reaches for net/smtp itself. Two concrete
// implementations live here:
//
//   - SMTPMailer dials a real SMTP relay (e.g. MailHog in dev, SES/SendGrid
//     SMTP in production) and is selected when Config.Host is set.
//   - LogMailer writes the rendered message to the structured log; it is the
//     dev/no-SMTP fallback so the app still boots when SMTP isn't configured.
//
// Adding a new provider later (SES native API, SendGrid HTTP) is a matter of
// implementing the Mailer interface — no caller code needs to change.
//
// # Decorator pattern
//
// Cross-cutting concerns around Send (retries, structured logging, metrics)
// are NOT baked into the SMTP/Log mailers. Instead this package ships three
// composable decorators — RetryingMailer, LoggingMailer, MetricsMailer — each
// of which embeds an inner Mailer, adds one behaviour, and exposes the same
// Send(ctx, msg) signature. They are wired together in main.go in the order
// that matches the desired behaviour: the innermost decorator runs closest
// to the wrapped implementation, the outermost runs first/last. The pattern
// is Go's idiomatic "embed an interface, add behaviour, return it" — it
// keeps every concern in one file, makes each one independently testable,
// and lets new concerns (rate limiting, circuit breaker, sampling) drop in
// without touching the SMTP code.
//
// See internal/mailer/Readme.md for the recommended composition order and a
// checklist for adding a new decorator.
package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer for the mailer package; each Send call gets its own span so the trace
// timeline shows whether an outbound email contributed to a slow request.
var tracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/internal/mailer")

// MessageKind describes what kind of email a Message is. It is propagated as
// a metric label (kind="order_confirmation", kind="password_reset", ...) so
// dashboards can group sent/failed counts by email template without leaking
// per-recipient cardinality. An empty Kind is recorded as "unknown".
type MessageKind = string

const (
	KindUnknown           MessageKind = "unknown"
	KindOrderConfirmation MessageKind = "order_confirmation"
	KindOrderShipped      MessageKind = "order_shipped"
	KindPasswordReset     MessageKind = "password_reset"
)

// Message is one outbound email. From is optional: when blank, the Mailer
// fills in its configured default sender. HTMLBody and TextBody are both
// optional but at least one MUST be set; when both are populated the message
// is built as multipart/alternative so clients pick the variant they render
// best.
type Message struct {
	To       string
	From     string
	Subject  string
	HTMLBody string
	TextBody string
	// Kind tags the email template ("order_confirmation",
	// "password_reset", ...). It is used only by the observability layer
	// to label the gocommerce_emails_sent_total counter — an empty Kind
	// is recorded as "unknown" and is otherwise harmless to the MIME
	// payload.
	Kind MessageKind
}

// kindOrDefault returns the configured Kind or "unknown" when blank, so the
// metric label always carries a small bounded vocabulary.
func kindOrDefault(k MessageKind) MessageKind {
	if k == "" {
		return KindUnknown
	}
	return k
}

// Mailer is the abstraction every caller in the codebase depends on. The
// concrete implementation is chosen at composition-root time via New(); call
// sites never type-assert to a specific implementation.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Config drives New(). When Host is empty we return a LogMailer instead of
// trying to dial — this is the local-development default so the app still
// boots without MailHog running. Username/Password are optional; when
// Username is "" we pass nil auth (MailHog accepts unauthenticated SMTP).
type Config struct {
	Host     string
	Username string
	Password string
	From     string
}

// New returns the Mailer implementation matching cfg: SMTPMailer when Host is
// set, LogMailer otherwise. The LogMailer fallback emits a single WARN at
// startup so an operator forgetting to wire SMTP in staging cannot miss it.
func New(cfg Config, logger logrus.FieldLogger) Mailer {
	if cfg.Host == "" {
		if logger != nil {
			logger.Warn("mailer: SMTP_HOST is empty; using LogMailer (emails will be logged, not sent)")
		}
		return &LogMailer{Logger: logger, From: cfg.From}
	}
	return &SMTPMailer{
		Host:     cfg.Host,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.From,
	}
}

// SMTPMailer delivers via plain net/smtp. The zero value is NOT usable: at a
// minimum Host and From must be set. Username/Password are optional.
type SMTPMailer struct {
	Host     string
	Username string
	Password string
	From     string
}

// ErrEmptyMessage is returned by Send when the supplied Message has no body
// at all. We refuse to dispatch a blank email rather than send something the
// recipient will ignore.
var ErrEmptyMessage = errors.New("mailer: message has no body")

// Send dispatches msg over SMTP. The MIME body is built locally so the
// implementation can produce a true multipart/alternative when both bodies
// are present without dragging in net/mail or net/textproto.
//
// Metrics: the emails_sent_total counter is NOT incremented here; that
// responsibility lives in the MetricsMailer decorator, which wraps this
// implementation in cmd/web/main.go. Keeping it out of the leaf mailer
// avoids double-counting once retries are layered in: the decorator records
// the final outcome (one observation per Message), not per attempt.
func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	kind := kindOrDefault(msg.Kind)
	ctx, span := tracer.Start(ctx, "Mailer.SMTP.Send", trace.WithAttributes(
		attribute.String("mail.kind", kind),
	))
	defer span.End()

	if msg.HTMLBody == "" && msg.TextBody == "" {
		span.RecordError(ErrEmptyMessage)
		span.SetStatus(codes.Error, ErrEmptyMessage.Error())
		return ErrEmptyMessage
	}
	if msg.To == "" {
		err := errors.New("mailer: To is required")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	from := msg.From
	if from == "" {
		from = m.From
	}

	body, err := buildMIME(from, msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("mailer: build MIME: %w", err)
	}

	// host is the SMTP host portion (without port) used by smtp.PlainAuth.
	// The full host:port string is what smtp.SendMail dials.
	host := m.Host
	if i := strings.IndexByte(host, ':'); i > 0 {
		host = host[:i]
	}

	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, host)
	}

	// net/smtp does not honor ctx natively, but we still respect a
	// pre-cancelled context so a Publish that fires during shutdown does
	// not block on the SMTP dial.
	if err := ctx.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err := smtp.SendMail(m.Host, auth, from, []string{msg.To}, body); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// buildMIME constructs the raw RFC822 bytes net/smtp expects. For
// HTML+text we emit a multipart/alternative; for text-only we emit a
// straight text/plain; for HTML-only we emit text/html.
func buildMIME(from string, msg Message) ([]byte, error) {
	var buf bytes.Buffer

	headers := func(extra map[string]string) {
		buf.WriteString("From: " + from + "\r\n")
		buf.WriteString("To: " + msg.To + "\r\n")
		buf.WriteString("Subject: " + msg.Subject + "\r\n")
		buf.WriteString("MIME-Version: 1.0\r\n")
		for k, v := range extra {
			buf.WriteString(k + ": " + v + "\r\n")
		}
	}

	switch {
	case msg.HTMLBody != "" && msg.TextBody != "":
		boundary, err := mimeBoundary()
		if err != nil {
			return nil, err
		}
		headers(map[string]string{
			"Content-Type": `multipart/alternative; boundary="` + boundary + `"`,
		})
		buf.WriteString("\r\n")
		// text part first — clients pick the LAST part they can
		// render, so HTML naturally wins when supported.
		buf.WriteString("--" + boundary + "\r\n")
		buf.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n\r\n")
		buf.WriteString(msg.TextBody)
		buf.WriteString("\r\n")
		buf.WriteString("--" + boundary + "\r\n")
		buf.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n\r\n")
		buf.WriteString(msg.HTMLBody)
		buf.WriteString("\r\n")
		buf.WriteString("--" + boundary + "--\r\n")
	case msg.HTMLBody != "":
		headers(map[string]string{
			"Content-Type": `text/html; charset="utf-8"`,
		})
		buf.WriteString("\r\n")
		buf.WriteString(msg.HTMLBody)
	default:
		headers(map[string]string{
			"Content-Type": `text/plain; charset="utf-8"`,
		})
		buf.WriteString("\r\n")
		buf.WriteString(msg.TextBody)
	}

	return buf.Bytes(), nil
}

// mimeBoundary returns a fresh, RFC 2046-safe boundary string. Sixteen
// random bytes hex-encoded yields a 32-char ASCII token, easily unique
// within one process and well under the 70-char limit.
func mimeBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gocommerce-" + hex.EncodeToString(b), nil
}

// LogMailer writes a structured log line for every send instead of
// delivering. Useful for local development and for any deployment that has
// not (yet) wired up an SMTP relay.
type LogMailer struct {
	Logger logrus.FieldLogger
	From   string
}

// Send writes the message metadata to the structured logger at info level.
// The body itself is intentionally NOT logged in full (passwords / reset
// tokens can show up inside it); a truncated preview is enough for a
// developer to confirm the right email was triggered.
//
// Metrics: the emails_sent_total counter is NOT incremented here; that
// responsibility lives in the MetricsMailer decorator (see metrics.go).
func (m *LogMailer) Send(ctx context.Context, msg Message) error {
	kind := kindOrDefault(msg.Kind)
	_, span := tracer.Start(ctx, "Mailer.Log.Send", trace.WithAttributes(
		attribute.String("mail.kind", kind),
	))
	defer span.End()

	if msg.To == "" {
		err := errors.New("mailer: To is required")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	from := msg.From
	if from == "" {
		from = m.From
	}
	preview := msg.TextBody
	if preview == "" {
		preview = msg.HTMLBody
	}
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	if m.Logger != nil {
		m.Logger.WithFields(logrus.Fields{
			"mailer":  "log",
			"from":    from,
			"to":      msg.To,
			"subject": msg.Subject,
			"preview": preview,
		}).Info("email (not actually sent)")
	}
	return nil
}
