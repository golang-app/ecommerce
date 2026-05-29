// Package integration holds the fulfillment context's published
// language: the integration events other bounded contexts may
// subscribe to. They are deliberately distinct from the internal
// fulfillment/domain events (which are an in-memory bridge to the bus
// only). The names are prefixed with "fulfillment." so they cannot
// collide with the checkout context's existing OrderShipped /
// OrderDelivered / OrderRefunded domain events (which live in
// checkout/domain and continue to be written to the event log for
// replay).
//
// Other contexts (e.g. a future email-on-shipment subscriber) wire
// onto these names without depending on checkout.
package integration

import "time"

// OrderShipped is published when a fulfillment transitions to
// shipped. Carrier + TrackingCode mirror what was recorded by Label.
type OrderShipped struct {
	OrderID      string
	Carrier      string
	TrackingCode string
	At           time.Time
}

// EventName returns the wire name used by the outbox decoder / bus
// subscribers. The "fulfillment." prefix makes the publishing context
// obvious from the name alone.
func (OrderShipped) EventName() string { return "fulfillment.OrderShipped" }

// OrderDelivered is published when a fulfillment reaches the delivered
// terminal state.
type OrderDelivered struct {
	OrderID string
	At      time.Time
}

func (OrderDelivered) EventName() string { return "fulfillment.OrderDelivered" }

// OrderRefunded is published when a fulfillment is refunded. Reason
// is the operator-supplied note recorded at refund time.
type OrderRefunded struct {
	OrderID string
	Reason  string
	At      time.Time
}

func (OrderRefunded) EventName() string { return "fulfillment.OrderRefunded" }
