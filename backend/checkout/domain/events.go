package domain

import "time"

// Event is a fact that happened to an order aggregate. Events are the source
// of truth; the read model is projected from them.
//
// Events are pure domain values — how they are serialised for the event store
// is the persistence layer's concern (see checkout/adapter).
type Event interface {
	EventType() string
	OccurredAt() time.Time
}

// OrderPlaced is emitted when a customer places an order. It carries the full
// snapshot of what was ordered so the order is self-contained.
type OrderPlaced struct {
	OrderID    string
	UserID     string
	CustomerID string
	ShipTo     Address
	ShipMethod ShippingMethod
	PayMethod  PaymentMethod
	Lines      []Line
	At         time.Time
}

func (e OrderPlaced) EventType() string     { return "OrderPlaced" }
func (e OrderPlaced) OccurredAt() time.Time { return e.At }

// PaymentSucceeded is emitted when the payment for an order is captured.
type PaymentSucceeded struct {
	OrderID string
	At      time.Time
}

func (e PaymentSucceeded) EventType() string     { return "PaymentSucceeded" }
func (e PaymentSucceeded) OccurredAt() time.Time { return e.At }

// PaymentFailed is emitted when the payment for an order is declined.
type PaymentFailed struct {
	OrderID string
	Reason  string
	At      time.Time
}

func (e PaymentFailed) EventType() string     { return "PaymentFailed" }
func (e PaymentFailed) OccurredAt() time.Time { return e.At }
