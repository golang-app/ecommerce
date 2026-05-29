package adapter

import (
	"encoding/json"
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
		Channel:        "web",
		At:             at,
	}

	typ, version, payload, err := marshalEvent(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if typ != "OrderPlaced" {
		t.Fatalf("type = %q, want OrderPlaced", typ)
	}
	if version != 2 {
		t.Fatalf("version = %d, want 2 (latest OrderPlaced schema)", version)
	}

	got, err := unmarshalEvent(typ, version, payload)
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
	if op.Channel != "web" {
		t.Errorf("channel round-trip mismatch: got %q want web", op.Channel)
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
		typ, version, payload, err := marshalEvent(e)
		if err != nil {
			t.Fatalf("marshal %s: %v", e.EventType(), err)
		}
		if version != 1 {
			t.Errorf("version for %s = %d, want 1 (event has not evolved yet)", typ, version)
		}
		got, err := unmarshalEvent(typ, version, payload)
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
		typ, version, payload, err := marshalEvent(e)
		if err != nil {
			t.Fatalf("marshal %s: %v", e.EventType(), err)
		}
		got, err := unmarshalEvent(typ, version, payload)
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
	typ, version, payload, err := marshalEvent(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalEvent(typ, version, payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

// TestUnmarshalOrderPlacedV1 feeds a hand-built v1 payload (no channel key)
// into unmarshalEvent and asserts the upcaster materialises Channel ==
// "unknown" on the resulting domain event. This is the textbook upcaster
// check: the aggregate's apply() should never see a v1 shape directly.
func TestUnmarshalOrderPlacedV1(t *testing.T) {
	at := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)

	// Hand-built v1 JSON — note the deliberate ABSENCE of the "channel"
	// key. This is exactly what rows written before the v2 migration look
	// like in the database.
	v1JSON := []byte(`{
		"order_id": "ord-v1",
		"user_id": "cart-1",
		"customer_id": "jane@example.com",
		"ship_to": {"name":"Jane","street1":"1 Main St","street2":"","city":"PDX","zip":"97201","country":"USA"},
		"ship_method": {"code":"courier","label":"Courier","cost":1500},
		"pay_method": {"code":"card","label":"Card"},
		"lines": [{"product_id":"ceramic-mug-cream","product_name":"Mug","qty":2,"price_amount":2400,"price_currency":"USD"}],
		"tax": 426,
		"shipping_cost": 0,
		"discount_code": "SAVE10",
		"discount_amount": 240,
		"at": "2026-05-27T09:00:00Z"
	}`)

	got, err := unmarshalEvent("OrderPlaced", 1, v1JSON)
	if err != nil {
		t.Fatalf("unmarshal v1: %v", err)
	}
	op, ok := got.(domain.OrderPlaced)
	if !ok {
		t.Fatalf("got %T, want domain.OrderPlaced", got)
	}
	if op.Channel != "unknown" {
		t.Errorf("upcast channel = %q, want unknown (v1 default fill)", op.Channel)
	}
	// Sanity: the rest of the payload survived the upcast unchanged.
	if op.OrderID != "ord-v1" || op.Tax != 426 || op.DiscountCode != "SAVE10" || op.DiscountAmount != 240 {
		t.Errorf("v1 payload not preserved: %+v", op)
	}
	if !op.At.Equal(at) {
		t.Errorf("v1 occurred-at lost: got %v want %v", op.At, at)
	}
	if len(op.Lines) != 1 || op.Lines[0].PriceAmount() != 2400 {
		t.Errorf("v1 lines not preserved: %+v", op.Lines)
	}
}

// TestRoundTripOrderPlacedV2 locks in that a v2 OrderPlaced (with Channel)
// marshals at version 2 and round-trips back through unmarshalEvent
// without going through the upcaster.
func TestRoundTripOrderPlacedV2(t *testing.T) {
	at := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	original := domain.OrderPlaced{
		OrderID:    "ord-2",
		UserID:     "cart-1",
		CustomerID: "jane@example.com",
		ShipTo:     domain.RebuildAddress("Jane", "1 Main St", "", "PDX", "97201", "USA"),
		ShipMethod: domain.RebuildShippingMethod("pickup", "Pickup", 0),
		PayMethod:  domain.RebuildPaymentMethod("card", "Card"),
		Lines:      []domain.Line{domain.NewLine("v1", "Mug", 1, 1500, "USD")},
		Channel:    "web",
		At:         at,
	}

	typ, version, payload, err := marshalEvent(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if typ != "OrderPlaced" {
		t.Fatalf("type = %q, want OrderPlaced", typ)
	}
	if version != 2 {
		t.Fatalf("version = %d, want 2", version)
	}

	// Confirm the wire payload actually carries the channel key — guards
	// against a future regression that silently drops the field.
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("re-parse payload: %v", err)
	}
	if raw["channel"] != "web" {
		t.Errorf("wire payload channel = %v, want web", raw["channel"])
	}

	got, err := unmarshalEvent(typ, version, payload)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	op, ok := got.(domain.OrderPlaced)
	if !ok {
		t.Fatalf("got %T, want domain.OrderPlaced", got)
	}
	if op.Channel != "web" {
		t.Errorf("channel lost in round-trip: got %q want web", op.Channel)
	}
}

// TestUnmarshalOrderPlacedUnknownVersion guards the codec contract: an
// unrecognised version is a loud failure, not a silent zero-value load.
func TestUnmarshalOrderPlacedUnknownVersion(t *testing.T) {
	if _, err := unmarshalEvent("OrderPlaced", 99, []byte(`{}`)); err == nil {
		t.Fatal("expected error for unknown OrderPlaced version, got nil")
	}
}
