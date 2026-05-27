package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestPlaceOrder_EmptyCartRejected(t *testing.T) {
	_, err := domain.PlaceOrder("ord-1", "cart-1", "", domain.Address{},
		domain.RebuildShippingMethod("pickup", "Personal pickup", 0),
		domain.RebuildPaymentMethod("cod", "Cash on delivery"),
		nil, time.Now())
	if !errors.Is(err, domain.ErrCartEmpty) {
		t.Errorf("err = %v, want ErrCartEmpty", err)
	}
}

func TestPlaceOrder_EmitsPendingOrderPlaced(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	lines := []domain.Line{
		domain.NewLine("v1", "Mug", 2, 1500, "USD"),
		domain.NewLine("v2", "Plate", 1, 2000, "USD"),
	}
	o, err := domain.PlaceOrder("ord-1", "cart-1", "jane@example.com", domain.Address{},
		domain.RebuildShippingMethod("courier", "Courier", 1500),
		domain.RebuildPaymentMethod("card", "Credit / debit card"),
		lines, at)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	if o.Status() != domain.StatusPending {
		t.Errorf("status = %q, want pending", o.Status())
	}
	// A freshly placed order has exactly one uncommitted event at sequence 1.
	pending := o.PendingEvents()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	if _, ok := pending[0].(domain.OrderPlaced); !ok {
		t.Errorf("pending[0] = %T, want OrderPlaced", pending[0])
	}
	if o.ExpectedVersion() != 0 {
		t.Errorf("ExpectedVersion = %d, want 0 (first event appends at seq 1)", o.ExpectedVersion())
	}
	// subtotal 2*1500 + 1*2000 = 5000, + 1500 shipping = 6500.
	if o.Subtotal() != 5000 {
		t.Errorf("subtotal = %d, want 5000", o.Subtotal())
	}
	if o.TotalAmount() != 6500 {
		t.Errorf("total = %d, want 6500", o.TotalAmount())
	}
	if o.TotalCurrency() != "USD" {
		t.Errorf("currency = %q, want USD", o.TotalCurrency())
	}
}

func TestLine_Totals(t *testing.T) {
	l := domain.NewLine("v1", "Mug", 3, 1234, "USD")
	if l.LineTotal() != 3702 {
		t.Errorf("LineTotal = %d, want 3702", l.LineTotal())
	}
	if l.LineTotalDisplay() != "37.02" {
		t.Errorf("LineTotalDisplay = %q, want 37.02", l.LineTotalDisplay())
	}
	if l.PriceDisplay() != "12.34" {
		t.Errorf("PriceDisplay = %q, want 12.34", l.PriceDisplay())
	}
}
