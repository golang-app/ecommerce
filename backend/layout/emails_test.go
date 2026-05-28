package layout

import (
	"strings"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
)

// TestRenderOrderConfirmation locks in that the embedded templates parse
// AND execute against a representative OrderView. A regression in either
// the template syntax or a renamed accessor on OrderView is caught here
// without spinning up a full integration test.
func TestRenderOrderConfirmation(t *testing.T) {
	view := checkoutQuery.NewOrderView(
		"ord-123",
		"alice@example.com",
		domain.Status("paid"),
		time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC),
		[]domain.Line{
			domain.NewLine("p-1", "Widget", 2, 999, "USD"),
		},
		domain.Address{},
		domain.RebuildShippingMethod("pickup", "Pickup", 0),
		domain.RebuildPaymentMethod("card", "Card"),
		1998,
		0,
		0,
		1998,
		"USD",
		"",
		"",
		"",
		0,
	)

	msg, err := RenderOrderConfirmation(view, "http://localhost:8080")
	if err != nil {
		t.Fatalf("RenderOrderConfirmation: %v", err)
	}
	if msg.To != "alice@example.com" {
		t.Fatalf("To = %q, want alice@example.com", msg.To)
	}
	if !strings.Contains(msg.Subject, "ord-123") {
		t.Fatalf("Subject does not include order id: %q", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "Widget") {
		t.Fatalf("HTMLBody missing item name: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.HTMLBody, "/order/ord-123") {
		t.Fatalf("HTMLBody missing OrderURL: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "Widget") {
		t.Fatalf("TextBody missing item name: %q", msg.TextBody)
	}
}

func TestRenderPasswordReset(t *testing.T) {
	msg, err := RenderPasswordReset("bob@example.com", "raw-token-abc", "http://localhost:8080/", 30)
	if err != nil {
		t.Fatalf("RenderPasswordReset: %v", err)
	}
	if msg.To != "bob@example.com" {
		t.Fatalf("To = %q, want bob@example.com", msg.To)
	}
	if !strings.Contains(msg.HTMLBody, "raw-token-abc") {
		t.Fatalf("HTMLBody missing token: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.HTMLBody, "/auth/reset?token=raw-token-abc") {
		t.Fatalf("HTMLBody missing ResetURL: %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "30") {
		t.Fatalf("TextBody missing TTL: %q", msg.TextBody)
	}
}
