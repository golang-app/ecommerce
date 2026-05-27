// Package integration holds the checkout context's published language: the
// integration events other bounded contexts may subscribe to. These are
// distinct from the internal, event-sourced domain events in
// checkout/domain — they are the stable contract checkout exposes outward.
package integration

import "time"

// OrderPaid is published when an order's payment succeeds. Subscribers react
// to it without checkout knowing about them — e.g. the cart context empties
// the basket the order was placed from.
type OrderPaid struct {
	OrderID    string
	SessionID  string // the cart/session id the order was placed from
	CustomerID string
	At         time.Time
}

func (OrderPaid) EventName() string { return "checkout.OrderPaid" }
