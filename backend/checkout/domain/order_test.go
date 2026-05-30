package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func paidOrderEvents() []domain.Event {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	return []domain.Event{
		domain.OrderPlaced{
			OrderID:    "ord-1",
			UserID:     "cart-1",
			CustomerID: "jane@example.com",
			ShipTo:     domain.RebuildAddress("Jane", "1 Main St", "", "PDX", "97201", "USA"),
			ShipMethod: domain.RebuildShippingMethod("courier", "Courier", 1500),
			PayMethod:  domain.RebuildPaymentMethod("card", "Credit / debit card"),
			Lines:      []domain.Line{domain.NewLine("v1", "Item", 2, 1000, "USD")},
			At:         at,
		},
		domain.PaymentSucceeded{OrderID: "ord-1", At: at},
	}
}

func TestRehydrate_FoldsToPaid(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())

	if o.Status() != domain.StatusPaid {
		t.Fatalf("status = %q, want paid", o.Status())
	}
	if len(o.PendingEvents()) != 0 {
		t.Errorf("rehydrated order should have no pending events, got %d", len(o.PendingEvents()))
	}
	// subtotal 2*1000 + 1500 shipping
	if o.TotalAmount() != 3500 {
		t.Errorf("total = %d, want 3500", o.TotalAmount())
	}
}

func TestCancel_RehydrateCommandAppend(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())

	if err := o.Cancel("changed mind", time.Now()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	if o.Status() != domain.StatusCancelled {
		t.Errorf("status = %q, want cancelled", o.Status())
	}
	pending := o.PendingEvents()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(pending))
	}
	if _, ok := pending[0].(domain.OrderCancelled); !ok {
		t.Errorf("pending event = %T, want OrderCancelled", pending[0])
	}
	// Two events were folded in; the cancellation must append at sequence 3.
	if o.ExpectedVersion() != 2 {
		t.Errorf("ExpectedVersion = %d, want 2 (cancellation appends at seq 3)", o.ExpectedVersion())
	}
}

func TestCancel_NotAllowedTwice(t *testing.T) {
	o := domain.RehydrateOrder(paidOrderEvents())
	if err := o.Cancel("first", time.Now()); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := o.Cancel("again", time.Now()); !errors.Is(err, domain.ErrOrderNotCancellable) {
		t.Errorf("second cancel err = %v, want ErrOrderNotCancellable", err)
	}
}

func TestCancel_FailedOrderNotCancellable(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	o := domain.RehydrateOrder([]domain.Event{
		domain.OrderPlaced{OrderID: "ord-2", Lines: []domain.Line{domain.NewLine("v1", "Item", 1, 1000, "USD")}, ShipMethod: domain.RebuildShippingMethod("pickup", "Personal pickup", 0), At: at},
		domain.PaymentFailed{OrderID: "ord-2", Reason: "declined", At: at},
	})

	if o.Status() != domain.StatusFailed {
		t.Fatalf("status = %q, want failed", o.Status())
	}
	if err := o.Cancel("nope", time.Now()); !errors.Is(err, domain.ErrOrderNotCancellable) {
		t.Errorf("cancel of failed order err = %v, want ErrOrderNotCancellable", err)
	}
}

// TestPlaceOrder_AppliesDiscount locks in the discount-before-tax pricing
// math: the OrderPlaced event carries the resolved discount and the
// rehydrated order's total reflects (subtotal - discount) + tax + shipping.
func TestPlaceOrder_AppliesDiscount(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	lines := []domain.Line{domain.NewLine("v1", "Mug", 4, 1000, "USD")} // subtotal 4000
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)

	// Caller resolved a 10% code: discount=400 on subtotal 4000;
	// discounted subtotal = 3600; caller computed tax 320 on it and kept
	// shipping at 1500. Total = 3600 + 320 + 1500 = 5420.
	o, err := domain.PlaceOrder("ord-d", "cart-1", "jane@example.com", domain.Address{}, method,
		domain.RebuildPaymentMethod("card", "Card"), lines, 320, 1500, "SAVE10", 400, "web", at)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if o.DiscountCode() != "SAVE10" || o.DiscountAmount() != 400 {
		t.Errorf("discount fields lost: code=%q amount=%d", o.DiscountCode(), o.DiscountAmount())
	}
	if o.TotalAmount() != 5420 {
		t.Errorf("total = %d, want 5420 (3600 discounted subtotal + 320 tax + 1500 shipping)", o.TotalAmount())
	}
}

// TestPlaceOrder_AppliesTaxAndEffectiveShipping locks in the tax/shipping
// math driven by the pluggable pricing strategies: the OrderPlaced event
// carries the derived numbers and the rehydrated order's total reflects
// them.
func TestPlaceOrder_AppliesTaxAndEffectiveShipping(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	lines := []domain.Line{domain.NewLine("v1", "Mug", 4, 1000, "USD")}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)

	// Caller computed an 8.875% tax on 4000 = 355 and free shipping (threshold met).
	o, err := domain.PlaceOrder("ord-3", "cart-1", "jane@example.com", domain.Address{}, method,
		domain.RebuildPaymentMethod("card", "Card"), lines, 355, 0, "", 0, "web", at)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if o.TaxAmount() != 355 {
		t.Errorf("tax = %d, want 355", o.TaxAmount())
	}
	if o.ShippingCost() != 0 {
		t.Errorf("shipping = %d, want 0 (free)", o.ShippingCost())
	}
	// subtotal 4000 + tax 355 + shipping 0.
	if o.TotalAmount() != 4355 {
		t.Errorf("total = %d, want 4355", o.TotalAmount())
	}
}

// TestApplyOrderPlaced_SetsChannel locks in the v2 OrderPlaced field
// behaviour: replaying an event whose Channel == "ios" lands on
// o.Channel() == "ios". The codec's upcaster guarantees apply() always
// sees a non-empty Channel; this test pins the aggregate side of that
// contract.
func TestApplyOrderPlaced_SetsChannel(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	o := domain.RehydrateOrder([]domain.Event{
		domain.OrderPlaced{
			OrderID:    "ord-ch",
			UserID:     "cart-1",
			ShipMethod: domain.RebuildShippingMethod("pickup", "Pickup", 0),
			PayMethod:  domain.RebuildPaymentMethod("card", "Card"),
			Lines:      []domain.Line{domain.NewLine("v1", "Mug", 1, 1500, "USD")},
			Channel:    "ios",
			At:         at,
		},
	})
	if o.Channel() != "ios" {
		t.Errorf("Channel() = %q, want ios", o.Channel())
	}
}

// TestFulfillmentTransitions exercises the paid → shipped → delivered
// happy path plus the guards: shipping a pending order fails, delivering a
// paid order fails, refunding a delivered order succeeds, and refunding a
// cancelled order fails.
func TestFulfillmentTransitions(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)

	// Paid order: ship/deliver/refund all valid.
	paid := domain.RehydrateOrder(paidOrderEvents())
	if err := paid.MarkShipped("UPS", "1Z-XYZ", at); err != nil {
		t.Fatalf("MarkShipped: %v", err)
	}
	if paid.Status() != domain.StatusShipped {
		t.Errorf("after MarkShipped: status = %q, want shipped", paid.Status())
	}
	if paid.Carrier() != "UPS" || paid.TrackingCode() != "1Z-XYZ" {
		t.Errorf("carrier/tracking not stored: %q / %q", paid.Carrier(), paid.TrackingCode())
	}
	if err := paid.MarkDelivered(at); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if paid.Status() != domain.StatusDelivered {
		t.Errorf("after MarkDelivered: status = %q, want delivered", paid.Status())
	}
	if err := paid.Refund("customer return", at); err != nil {
		t.Fatalf("Refund on delivered: %v", err)
	}
	if paid.Status() != domain.StatusRefunded {
		t.Errorf("after Refund: status = %q, want refunded", paid.Status())
	}

	// Pending order: not shippable.
	pending := domain.RehydrateOrder([]domain.Event{
		domain.OrderPlaced{OrderID: "ord-p", Lines: []domain.Line{domain.NewLine("v", "X", 1, 100, "USD")}, ShipMethod: domain.RebuildShippingMethod("pickup", "Pickup", 0), At: at},
	})
	if err := pending.MarkShipped("UPS", "", at); !errors.Is(err, domain.ErrOrderNotShippable) {
		t.Errorf("MarkShipped on pending = %v, want ErrOrderNotShippable", err)
	}

	// Paid (not shipped) order: cannot be delivered yet.
	paid2 := domain.RehydrateOrder(paidOrderEvents())
	if err := paid2.MarkDelivered(at); !errors.Is(err, domain.ErrOrderNotDeliverable) {
		t.Errorf("MarkDelivered on paid = %v, want ErrOrderNotDeliverable", err)
	}

	// Cancelled order: not refundable.
	cancelled := domain.RehydrateOrder(paidOrderEvents())
	if err := cancelled.Cancel("changed mind", at); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := cancelled.Refund("late", at); !errors.Is(err, domain.ErrOrderNotRefundable) {
		t.Errorf("Refund on cancelled = %v, want ErrOrderNotRefundable", err)
	}
}
