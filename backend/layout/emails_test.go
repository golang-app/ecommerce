package layout

import (
	"strings"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentIntegration "github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
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

// TestRenderOrderShipped locks in the ECST property of the
// order-shipped email: every byte of the rendered bodies must come
// from the OrderShippedECST event payload supplied to the helper.
// The fixture is constructed inline (not loaded from any store), so a
// passing render proves the subscriber NEVER needs to call back into
// checkout's read side. A regression that re-introduces a checkout
// lookup would manifest as a missing field below.
func TestRenderOrderShipped(t *testing.T) {
	event := fulfillmentIntegration.OrderShippedECST{
		OrderID:      "ord-789",
		CustomerID:   "carol@example.com",
		Email:        "carol@example.com",
		Carrier:      "UPS",
		TrackingCode: "1Z-ABC-999",
		ShipTo: fulfillmentIntegration.ShippingAddressDTO{
			Name:    "Carol Example",
			Street1: "12 Test Lane",
			Street2: "Apt 4B",
			City:    "Krakow",
			Zip:     "30-000",
			Country: "PL",
		},
		Items: []fulfillmentIntegration.LineDTO{
			{ProductID: "p-1", ProductName: "Widget", Quantity: 2, PriceAmount: 999, PriceCurrency: "USD"},
			{ProductID: "p-2", ProductName: "Sprocket", Quantity: 1, PriceAmount: 2599, PriceCurrency: "USD"},
		},
		Subtotal:     4597,
		Tax:          0,
		ShippingCost: 500,
		Total:        5097,
		Currency:     "USD",
		At:           time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
	}

	msg, err := RenderOrderShipped(event)
	if err != nil {
		t.Fatalf("RenderOrderShipped: %v", err)
	}
	if msg.To != "carol@example.com" {
		t.Fatalf("To = %q, want carol@example.com", msg.To)
	}
	if !strings.Contains(msg.Subject, "ord-789") {
		t.Fatalf("Subject does not include order id: %q", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "shipped") {
		t.Fatalf("Subject does not include 'shipped': %q", msg.Subject)
	}

	// Every load-bearing field MUST show up in BOTH bodies. The
	// loop names the field being asserted so a failure points
	// straight at the missing piece in the template.
	wants := []struct {
		label string
		want  string
	}{
		{"OrderID", "ord-789"},
		{"Carrier", "UPS"},
		{"TrackingCode", "1Z-ABC-999"},
		{"ShipTo.Name", "Carol Example"},
		{"ShipTo.Street1", "12 Test Lane"},
		{"ShipTo.City", "Krakow"},
		{"ShipTo.Country", "PL"},
		{"item 1 name", "Widget"},
		{"item 2 name", "Sprocket"},
	}
	for _, w := range wants {
		if !strings.Contains(msg.HTMLBody, w.want) {
			t.Errorf("HTMLBody missing %s (%q)", w.label, w.want)
		}
		if !strings.Contains(msg.TextBody, w.want) {
			t.Errorf("TextBody missing %s (%q)", w.label, w.want)
		}
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
