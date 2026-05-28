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
//
// Tax is the tax amount in minor units computed at place time from the
// configured tax rate; ShippingCost is the EFFECTIVE shipping price at place
// time (which may be 0 if the configured free-shipping threshold was met,
// overriding the catalogue method's Cost()).
type OrderPlaced struct {
	OrderID      string
	UserID       string
	CustomerID   string
	ShipTo       Address
	ShipMethod   ShippingMethod
	PayMethod    PaymentMethod
	Lines        []Line
	Tax          int64
	ShippingCost int64
	At           time.Time
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

// OrderCancelled is emitted when a placed order is cancelled by the customer.
type OrderCancelled struct {
	OrderID string
	Reason  string
	At      time.Time
}

func (e OrderCancelled) EventType() string     { return "OrderCancelled" }
func (e OrderCancelled) OccurredAt() time.Time { return e.At }

// OrderShipped is emitted when an admin marks a paid order as dispatched.
// Carrier and TrackingCode are optional (empty strings when not supplied).
type OrderShipped struct {
	OrderID      string
	Carrier      string
	TrackingCode string
	At           time.Time
}

func (e OrderShipped) EventType() string     { return "OrderShipped" }
func (e OrderShipped) OccurredAt() time.Time { return e.At }

// OrderDelivered is emitted when an admin marks a shipped order as delivered.
type OrderDelivered struct {
	OrderID string
	At      time.Time
}

func (e OrderDelivered) EventType() string     { return "OrderDelivered" }
func (e OrderDelivered) OccurredAt() time.Time { return e.At }

// OrderRefunded is emitted when an admin refunds a paid/shipped/delivered
// order. Refund returns the goods so reserved/sold stock is added back to
// the catalogue.
type OrderRefunded struct {
	OrderID string
	Reason  string
	At      time.Time
}

func (e OrderRefunded) EventType() string     { return "OrderRefunded" }
func (e OrderRefunded) OccurredAt() time.Time { return e.At }
