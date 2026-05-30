package layout

import (
	"bytes"
	"embed"
	htmltmpl "html/template"
	"strings"
	"sync"
	texttmpl "text/template"

	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentIntegration "github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
)

// emailTemplates holds the embedded HTML/text bodies for every transactional
// email this app sends. Embedding them keeps the alpine runtime image working
// without bind-mounting tmpl/ — the static.go embed precedent already in this
// package solves the same problem for /static.
//
//go:embed tmpl/emails/*
var emailTemplates embed.FS

// siteName is repeated in several email bodies; centralising it here keeps
// the strings consistent and the templates short.
const emailSiteName = "GoCommerce"

// Parsed templates are lazy-loaded via sync.Once so the first send pays the
// parse cost and every subsequent one is a cheap Execute. Falling back to a
// startup-time parse would also work, but the Once form keeps the template
// graph next to the renderer that uses it and avoids touching layout.New's
// already-busy signature.
var (
	orderConfHTMLOnce sync.Once
	orderConfHTML     *htmltmpl.Template
	orderConfHTMLErr  error

	orderConfTextOnce sync.Once
	orderConfText     *texttmpl.Template
	orderConfTextErr  error

	resetHTMLOnce sync.Once
	resetHTML     *htmltmpl.Template
	resetHTMLErr  error

	resetTextOnce sync.Once
	resetText     *texttmpl.Template
	resetTextErr  error

	orderShippedHTMLOnce sync.Once
	orderShippedHTML     *htmltmpl.Template
	orderShippedHTMLErr  error

	orderShippedTextOnce sync.Once
	orderShippedText     *texttmpl.Template
	orderShippedTextErr  error
)

func loadOrderConfHTML() (*htmltmpl.Template, error) {
	orderConfHTMLOnce.Do(func() {
		orderConfHTML, orderConfHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_confirmation.html.tmpl")
	})
	return orderConfHTML, orderConfHTMLErr
}

func loadOrderConfText() (*texttmpl.Template, error) {
	orderConfTextOnce.Do(func() {
		orderConfText, orderConfTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_confirmation.txt.tmpl")
	})
	return orderConfText, orderConfTextErr
}

func loadResetHTML() (*htmltmpl.Template, error) {
	resetHTMLOnce.Do(func() {
		resetHTML, resetHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/password_reset.html.tmpl")
	})
	return resetHTML, resetHTMLErr
}

func loadResetText() (*texttmpl.Template, error) {
	resetTextOnce.Do(func() {
		resetText, resetTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/password_reset.txt.tmpl")
	})
	return resetText, resetTextErr
}

func loadOrderShippedHTML() (*htmltmpl.Template, error) {
	orderShippedHTMLOnce.Do(func() {
		orderShippedHTML, orderShippedHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_shipped.html.tmpl")
	})
	return orderShippedHTML, orderShippedHTMLErr
}

func loadOrderShippedText() (*texttmpl.Template, error) {
	orderShippedTextOnce.Do(func() {
		orderShippedText, orderShippedTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_shipped.txt.tmpl")
	})
	return orderShippedText, orderShippedTextErr
}

// RenderOrderConfirmation builds a Message for the order-paid email. The
// caller (the OrderPaid subscriber in cmd/web/main.go) supplies the order
// detail view and the storefront baseURL; this helper renders the two
// bodies and hands back a ready-to-send Message — it does NOT call mailer
// itself, keeping rendering and delivery cleanly separable.
func RenderOrderConfirmation(view checkoutQuery.OrderView, baseURL string) (mailer.Message, error) {
	htmlT, err := loadOrderConfHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderConfText()
	if err != nil {
		return mailer.Message{}, err
	}

	data := map[string]any{
		"Order":    view,
		"SiteName": emailSiteName,
		"OrderURL": strings.TrimRight(baseURL, "/") + "/order/" + view.ID(),
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := htmlT.Execute(&htmlBuf, data); err != nil {
		return mailer.Message{}, err
	}
	if err := textT.Execute(&textBuf, data); err != nil {
		return mailer.Message{}, err
	}

	return mailer.Message{
		To:       view.CustomerID(),
		Subject:  "Your order " + view.ID() + " is confirmed",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderConfirmation,
	}, nil
}

// RenderOrderShipped builds a Message for the fulfillment.OrderShippedECST
// integration event. It is the ECST companion to
// RenderOrderConfirmation: every byte the templates execute against
// comes from the event itself — there is NO callback into checkout's
// read side. The subscriber that drives this helper can therefore
// stay live even when the checkout context is unavailable, which is
// the whole point of the Event-Carried State Transfer pattern (see
// the fulfillment/integration package doc for the trade-off).
func RenderOrderShipped(event fulfillmentIntegration.OrderShippedECST) (mailer.Message, error) {
	htmlT, err := loadOrderShippedHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderShippedText()
	if err != nil {
		return mailer.Message{}, err
	}

	data := map[string]any{
		"Event":    event,
		"SiteName": emailSiteName,
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := htmlT.Execute(&htmlBuf, data); err != nil {
		return mailer.Message{}, err
	}
	if err := textT.Execute(&textBuf, data); err != nil {
		return mailer.Message{}, err
	}

	return mailer.Message{
		To:       event.Email,
		Subject:  "Your order " + event.OrderID + " has shipped",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderShipped,
	}, nil
}

// RenderPasswordReset builds a Message for the forgot-password flow. The
// raw token (NOT its hash) is embedded into the reset URL the recipient
// clicks; ttlMinutes is rendered into the body so the user knows how long
// they have.
func RenderPasswordReset(toEmail, rawToken, baseURL string, ttlMinutes int) (mailer.Message, error) {
	htmlT, err := loadResetHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadResetText()
	if err != nil {
		return mailer.Message{}, err
	}

	resetURL := strings.TrimRight(baseURL, "/") + "/auth/reset?token=" + rawToken
	data := map[string]any{
		"SiteName":   emailSiteName,
		"ResetURL":   resetURL,
		"TTLMinutes": ttlMinutes,
	}

	var htmlBuf, textBuf bytes.Buffer
	if err := htmlT.Execute(&htmlBuf, data); err != nil {
		return mailer.Message{}, err
	}
	if err := textT.Execute(&textBuf, data); err != nil {
		return mailer.Message{}, err
	}

	return mailer.Message{
		To:       toEmail,
		Subject:  "Reset your " + emailSiteName + " password",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindPasswordReset,
	}, nil
}
