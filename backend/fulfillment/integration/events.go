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
//
// NOTIFICATION vs EVENT-CARRIED STATE TRANSFER (ECST)
//
// This codebase deliberately demonstrates BOTH integration-event
// styles side-by-side so the trade-off is visible at a glance:
//
//   - NOTIFICATION events carry only identifiers (and a timestamp).
//     Subscribers learn that "something happened" and call back into
//     the publishing context's read side for the data they need.
//     OrderShipped (below) is the example: it carries OrderID +
//     Carrier + TrackingCode + At. A subscriber that wants the
//     line-items or the shipping address has to query checkout's
//     OrderView itself. Pros: tiny payload, the publisher's schema
//     stays small, no risk of stale denormalised state on the
//     consumer. Cons: every consumer takes a runtime dependency on
//     the publisher's read side (a coupling the bus was supposed to
//     remove), and back-pressure / outage on the publisher cascades
//     into the consumer.
//
//   - EVENT-CARRIED STATE TRANSFER (ECST) events carry the full
//     snapshot the consumer needs to do its job. OrderShippedECST
//     (below) is the example: it carries the customer's email, the
//     shipping address, the line items, and the totals — everything
//     a "your order has shipped" email needs to render without ever
//     calling back into checkout. Pros: consumers stay live even
//     when the publisher is down; the consumer never holds a stale
//     join against the publisher's read model. Cons: the published
//     payload is larger and the schema is a real contract (renaming
//     a field is now a breaking change for every subscriber).
//
// When to use each:
//
//   - Use a NOTIFICATION event when the subscriber's reaction is
//     small ("clear the cart") or when the subscriber always wants
//     the freshest data ("admin dashboard re-render").
//   - Use an ECST event when the subscriber wants to be operationally
//     independent from the publisher (e.g. a "ship notification"
//     email service that must not fail because checkout is hot) and
//     the snapshot the consumer needs is well-bounded.
//
// The published-language DTOs below (ShippingAddressDTO, LineDTO) are
// the CONTRACT for ECST consumers. Downstream consumers MUST bind to
// these DTOs and MUST NOT import checkout's internal value objects
// (checkout/domain.Address, checkout/domain.Line) — those are
// checkout's internal model and can change without notice; the DTOs
// in this file are the stable interchange shape.
package integration

import "time"

// OrderShipped is published when a fulfillment transitions to
// shipped. Carrier + TrackingCode mirror what was recorded by Label.
//
// NOTIFICATION-STYLE EVENT (see package doc). The payload carries the
// identifiers and the operational metadata recorded by the shipping
// transition; subscribers wanting more (line items, totals, the
// customer's address) must query the checkout read side themselves.
// Pair this with OrderShippedECST below for the contrast.
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

// ShippingAddressDTO is the published-language shape of a shipping
// address as it appears on the wire of ECST events. It deliberately
// duplicates the field shape of checkout/domain.Address rather than
// re-exporting it: downstream consumers bind to this DTO and never
// import checkout's internal types.
type ShippingAddressDTO struct {
	Name    string
	Street1 string
	Street2 string
	City    string
	Zip     string
	Country string
}

// LineDTO is the published-language shape of a single order line as
// it appears on the wire of ECST events. PriceAmount is in minor
// units of PriceCurrency (the same convention used everywhere else in
// the codebase).
type LineDTO struct {
	ProductID     string
	ProductName   string
	Quantity      int
	PriceAmount   int64
	PriceCurrency string
}

// OrderShippedECST is published when a fulfillment transitions to
// shipped, alongside the notification-style OrderShipped above.
//
// EVENT-CARRIED STATE TRANSFER (see package doc). The payload carries
// the full state a downstream consumer needs to render a "your order
// has shipped" notification — customer email, shipping destination,
// line items, totals — without calling back into checkout's read
// side. The notification-style OrderShipped above remains the durable
// signal recorded on the bus; this ECST event is published alongside
// it for consumers who want to be operationally independent of
// checkout.
type OrderShippedECST struct {
	OrderID      string
	CustomerID   string
	Email        string
	Carrier      string
	TrackingCode string
	ShipTo       ShippingAddressDTO
	Items        []LineDTO
	Subtotal     int64
	Tax          int64
	ShippingCost int64
	Total        int64
	Currency     string
	At           time.Time
}

// EventName returns the wire name used by the outbox decoder / bus
// subscribers. The "ECST" suffix in the name makes the style explicit
// at the subscription site so it is impossible to mistake for the
// notification-style OrderShipped.
func (OrderShippedECST) EventName() string { return "fulfillment.OrderShippedECST" }
