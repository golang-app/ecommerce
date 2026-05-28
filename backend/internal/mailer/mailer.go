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

	"github.com/sirupsen/logrus"
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
func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	if msg.HTMLBody == "" && msg.TextBody == "" {
		return ErrEmptyMessage
	}
	if msg.To == "" {
		return errors.New("mailer: To is required")
	}

	from := msg.From
	if from == "" {
		from = m.From
	}

	body, err := buildMIME(from, msg)
	if err != nil {
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
		return err
	}

	return smtp.SendMail(m.Host, auth, from, []string{msg.To}, body)
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
func (m *LogMailer) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("mailer: To is required")
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
