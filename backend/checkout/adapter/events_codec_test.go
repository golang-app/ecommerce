package adapter

import (
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func TestEventCodecRoundTrip_OrderPlaced(t *testing.T) {
	at := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	original := domain.OrderPlaced{
		OrderID:    "ord-1",
		UserID:     "cart-1",
		CustomerID: "jane@example.com",
		ShipTo:     domain.RebuildAddress("Jane", "1 Main St", "Apt 2", "PDX", "97201", "USA"),
		ShipMethod: domain.RebuildShippingMethod("courier", "Courier", 1500),
		PayMethod:  domain.RebuildPaymentMethod("card", "Credit / debit card"),
		Lines: []domain.Line{
			domain.NewLine("ceramic-mug-cream", "Ceramic Mug — Cream", 2, 2400, "USD"),
		},
		Tax:            426,
		ShippingCost:   0,
		DiscountCode:   "SAVE10",
		DiscountAmount: 240,
		At:             at,
	}

	typ, payload, err := marshalEvent(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if typ != "OrderPlaced" {
		t.Fatalf("type = %q, want OrderPlaced", typ)
	}

	got, err := unmarshalEvent(typ, payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	op, ok := got.(domain.OrderPlaced)
	if !ok {
		t.Fatalf("got %T, want domain.OrderPlaced", got)
	}

	if op.OrderID != original.OrderID || op.UserID != original.UserID || op.CustomerID != original.CustomerID {
		t.Errorf("identity mismatch: %+v", op)
	}
	if !op.At.Equal(original.At) {
		t.Errorf("placed-at mismatch: got %v want %v", op.At, original.At)
	}
	if op.ShipTo.City() != "PDX" || op.ShipTo.Street2() != "Apt 2" {
		t.Errorf("address round-trip mismatch: %+v", op.ShipTo)
	}
	if op.ShipMethod.Code() != "courier" || op.ShipMethod.Cost() != 1500 {
		t.Errorf("shipping method round-trip mismatch: %+v", op.ShipMethod)
	}
	if op.PayMethod.Code() != "card" {
		t.Errorf("payment method round-trip mismatch: %+v", op.PayMethod)
	}
	if len(op.Lines) != 1 || op.Lines[0].ProductName() != "Ceramic Mug — Cream" || op.Lines[0].Quantity() != 2 || op.Lines[0].PriceAmount() != 2400 {
		t.Errorf("line round-trip mismatch: %+v", op.Lines)
	}
	if op.Tax != 426 || op.ShippingCost != 0 {
		t.Errorf("tax/shipping round-trip mismatch: tax=%d shipping=%d", op.Tax, op.ShippingCost)
	}
	if op.DiscountCode != "SAVE10" || op.DiscountAmount != 240 {
		t.Errorf("discount round-trip mismatch: code=%q amount=%d", op.DiscountCode, op.DiscountAmount)
	}
}

func TestEventCodecRoundTrip_Fulfillment(t *testing.T) {
	at := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	cases := []domain.Event{
		domain.OrderShipped{OrderID: "ord-1", Carrier: "UPS", TrackingCode: "1Z-XYZ", At: at},
		domain.OrderDelivered{OrderID: "ord-1", At: at},
		domain.OrderRefunded{OrderID: "ord-1", Reason: "customer return", At: at},
	}
	for _, e := range cases {
		typ, payload, err := marshalEvent(e)
		if err != nil {
			t.Fatalf("marshal %s: %v", e.EventType(), err)
		}
		got, err := unmarshalEvent(typ, payload)
		if err != nil {
			t.Fatalf("unmarshal %s: %v", typ, err)
		}
		if got.EventType() != e.EventType() {
			t.Errorf("type mismatch: got %s want %s", got.EventType(), e.EventType())
		}
		if !got.OccurredAt().Equal(at) {
			t.Errorf("occurred-at mismatch for %s", typ)
		}
	}

	// Carrier/tracking specifically need to survive the round-trip on OrderShipped.
	round := mustUnmarshal(t, domain.OrderShipped{OrderID: "ord-1", Carrier: "DHL", TrackingCode: "JJD", At: at})
	if os, ok := round.(domain.OrderShipped); !ok || os.Carrier != "DHL" || os.TrackingCode != "JJD" {
		t.Errorf("OrderShipped carrier/tracking lost: %+v", round)
	}
}

func TestEventCodecRoundTrip_Payment(t *testing.T) {
	at := time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC)

	for _, e := range []domain.Event{
		domain.PaymentSucceeded{OrderID: "ord-1", At: at},
		domain.PaymentFailed{OrderID: "ord-1", Reason: "declined", At: at},
	} {
		typ, payload, err := marshalEvent(e)
		if err != nil {
			t.Fatalf("marshal %s: %v", e.EventType(), err)
		}
		got, err := unmarshalEvent(typ, payload)
		if err != nil {
			t.Fatalf("unmarshal %s: %v", typ, err)
		}
		if got.EventType() != e.EventType() {
			t.Errorf("type mismatch: got %s want %s", got.EventType(), e.EventType())
		}
		if !got.OccurredAt().Equal(at) {
			t.Errorf("occurred-at mismatch for %s", typ)
		}
	}

	if pf, ok := mustUnmarshal(t, domain.PaymentFailed{OrderID: "ord-2", Reason: "no funds", At: at}).(domain.PaymentFailed); !ok || pf.Reason != "no funds" {
		t.Errorf("PaymentFailed reason not preserved: %+v", pf)
	}
}

func mustUnmarshal(t *testing.T, e domain.Event) domain.Event {
	t.Helper()
	typ, payload, err := marshalEvent(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalEvent(typ, payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}
