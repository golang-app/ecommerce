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

	orderPlacedHTMLOnce sync.Once
	orderPlacedHTML     *htmltmpl.Template
	orderPlacedHTMLErr  error

	orderPlacedTextOnce sync.Once
	orderPlacedText     *texttmpl.Template
	orderPlacedTextErr  error

	orderCancelledHTMLOnce sync.Once
	orderCancelledHTML     *htmltmpl.Template
	orderCancelledHTMLErr  error

	orderCancelledTextOnce sync.Once
	orderCancelledText     *texttmpl.Template
	orderCancelledTextErr  error

	paymentFailedHTMLOnce sync.Once
	paymentFailedHTML     *htmltmpl.Template
	paymentFailedHTMLErr  error

	paymentFailedTextOnce sync.Once
	paymentFailedText     *texttmpl.Template
	paymentFailedTextErr  error

	orderDeliveredHTMLOnce sync.Once
	orderDeliveredHTML     *htmltmpl.Template
	orderDeliveredHTMLErr  error

	orderDeliveredTextOnce sync.Once
	orderDeliveredText     *texttmpl.Template
	orderDeliveredTextErr  error

	orderRefundedHTMLOnce sync.Once
	orderRefundedHTML     *htmltmpl.Template
	orderRefundedHTMLErr  error

	orderRefundedTextOnce sync.Once
	orderRefundedText     *texttmpl.Template
	orderRefundedTextErr  error
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

func loadOrderPlacedHTML() (*htmltmpl.Template, error) {
	orderPlacedHTMLOnce.Do(func() {
		orderPlacedHTML, orderPlacedHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_placed.html.tmpl")
	})
	return orderPlacedHTML, orderPlacedHTMLErr
}

func loadOrderPlacedText() (*texttmpl.Template, error) {
	orderPlacedTextOnce.Do(func() {
		orderPlacedText, orderPlacedTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_placed.txt.tmpl")
	})
	return orderPlacedText, orderPlacedTextErr
}

func loadOrderCancelledHTML() (*htmltmpl.Template, error) {
	orderCancelledHTMLOnce.Do(func() {
		orderCancelledHTML, orderCancelledHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_cancelled.html.tmpl")
	})
	return orderCancelledHTML, orderCancelledHTMLErr
}

func loadOrderCancelledText() (*texttmpl.Template, error) {
	orderCancelledTextOnce.Do(func() {
		orderCancelledText, orderCancelledTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_cancelled.txt.tmpl")
	})
	return orderCancelledText, orderCancelledTextErr
}

func loadPaymentFailedHTML() (*htmltmpl.Template, error) {
	paymentFailedHTMLOnce.Do(func() {
		paymentFailedHTML, paymentFailedHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_payment_failed.html.tmpl")
	})
	return paymentFailedHTML, paymentFailedHTMLErr
}

func loadPaymentFailedText() (*texttmpl.Template, error) {
	paymentFailedTextOnce.Do(func() {
		paymentFailedText, paymentFailedTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_payment_failed.txt.tmpl")
	})
	return paymentFailedText, paymentFailedTextErr
}

func loadOrderDeliveredHTML() (*htmltmpl.Template, error) {
	orderDeliveredHTMLOnce.Do(func() {
		orderDeliveredHTML, orderDeliveredHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_delivered.html.tmpl")
	})
	return orderDeliveredHTML, orderDeliveredHTMLErr
}

func loadOrderDeliveredText() (*texttmpl.Template, error) {
	orderDeliveredTextOnce.Do(func() {
		orderDeliveredText, orderDeliveredTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_delivered.txt.tmpl")
	})
	return orderDeliveredText, orderDeliveredTextErr
}

func loadOrderRefundedHTML() (*htmltmpl.Template, error) {
	orderRefundedHTMLOnce.Do(func() {
		orderRefundedHTML, orderRefundedHTMLErr = htmltmpl.ParseFS(emailTemplates, "tmpl/emails/order_refunded.html.tmpl")
	})
	return orderRefundedHTML, orderRefundedHTMLErr
}

func loadOrderRefundedText() (*texttmpl.Template, error) {
	orderRefundedTextOnce.Do(func() {
		orderRefundedText, orderRefundedTextErr = texttmpl.ParseFS(emailTemplates, "tmpl/emails/order_refunded.txt.tmpl")
	})
	return orderRefundedText, orderRefundedTextErr
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

// RenderOrderPlaced builds a Message for the order placed pending payment email.
func RenderOrderPlaced(view checkoutQuery.OrderView, baseURL string) (mailer.Message, error) {
	htmlT, err := loadOrderPlacedHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderPlacedText()
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
		Subject:  "Your order " + view.ID() + " has been placed",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderPlaced,
	}, nil
}

// RenderOrderCancelled builds a Message for the order cancelled email.
func RenderOrderCancelled(view checkoutQuery.OrderView, reason, baseURL string) (mailer.Message, error) {
	htmlT, err := loadOrderCancelledHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderCancelledText()
	if err != nil {
		return mailer.Message{}, err
	}

	data := map[string]any{
		"Order":    view,
		"Reason":   reason,
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
		Subject:  "Your order " + view.ID() + " has been cancelled",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderCancelled,
	}, nil
}

// RenderPaymentFailed builds a Message for the payment failed email.
func RenderPaymentFailed(view checkoutQuery.OrderView, reason, baseURL string) (mailer.Message, error) {
	htmlT, err := loadPaymentFailedHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadPaymentFailedText()
	if err != nil {
		return mailer.Message{}, err
	}

	data := map[string]any{
		"Order":    view,
		"Reason":   reason,
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
		Subject:  "Payment failed for order " + view.ID(),
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderPaymentFailed,
	}, nil
}

// RenderOrderDelivered builds a Message for the order delivered email.
func RenderOrderDelivered(view checkoutQuery.OrderView, baseURL string) (mailer.Message, error) {
	htmlT, err := loadOrderDeliveredHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderDeliveredText()
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
		Subject:  "Your order " + view.ID() + " has been delivered",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderDelivered,
	}, nil
}

// RenderOrderRefunded builds a Message for the order refunded email.
func RenderOrderRefunded(view checkoutQuery.OrderView, reason, baseURL string) (mailer.Message, error) {
	htmlT, err := loadOrderRefundedHTML()
	if err != nil {
		return mailer.Message{}, err
	}
	textT, err := loadOrderRefundedText()
	if err != nil {
		return mailer.Message{}, err
	}

	data := map[string]any{
		"Order":    view,
		"Reason":   reason,
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
		Subject:  "Your order " + view.ID() + " has been refunded",
		HTMLBody: htmlBuf.String(),
		TextBody: textBuf.String(),
		Kind:     mailer.KindOrderRefunded,
	}, nil
}

