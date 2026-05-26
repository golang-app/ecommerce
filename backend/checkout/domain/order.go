package domain

import (
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusPaid    Status = "paid"
	StatusFailed  Status = "failed"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrCartEmpty     = errors.New("cannot place order from an empty cart")
)

// Order is a placed (or attempted) checkout. Once created, line items are a
// snapshot of the cart at order time — later changes to products or prices
// do not retroactively change the order.
//
// userID is the cart_id cookie value at order time (anonymous identifier).
// customerID is the authenticated customer (e.g. email) and is empty for
// orders placed without logging in. Only orders with a non-empty
// customerID show up in the per-user order history.
type Order struct {
	id          string
	userID      string
	customerID  string
	shipTo      Address
	shipMethod  ShippingMethod
	payMethod   PaymentMethod
	items       []Line
	subtotalAmt int64
	totalAmt    int64
	totalCcy    string
	status      Status
	placedAt    time.Time

	version       int
	pendingEvents []Event
}

// Line is a snapshot of a single cart line at order time.
type Line struct {
	productID     string
	productName   string
	qty           int
	priceAmount   int64
	priceCurrency string
}

func NewLine(productID, productName string, qty int, priceAmount int64, priceCurrency string) Line {
	return Line{
		productID:     productID,
		productName:   productName,
		qty:           qty,
		priceAmount:   priceAmount,
		priceCurrency: priceCurrency,
	}
}

func (l Line) ProductID() string     { return l.productID }
func (l Line) ProductName() string   { return l.productName }
func (l Line) Quantity() int         { return l.qty }
func (l Line) PriceAmount() int64    { return l.priceAmount }
func (l Line) PriceCurrency() string { return l.priceCurrency }
func (l Line) PriceDisplay() string  { return money(l.priceAmount) }
func (l Line) LineTotal() int64      { return l.priceAmount * int64(l.qty) }
func (l Line) LineTotalDisplay() string {
	return money(l.LineTotal())
}

// NewOrder builds an Order, deriving the total from the supplied lines and
// using the first line's currency. Callers are expected to ensure all lines
// share a currency (the cart bounded context enforces this on its side once
// the cross-currency safety fix lands).
func NewOrder(id, userID, customerID string, shipTo Address, shipMethod ShippingMethod, payMethod PaymentMethod, items []Line, status Status, placedAt time.Time) Order {
	var subtotal int64
	var ccy string
	for _, ln := range items {
		subtotal += ln.LineTotal()
		if ccy == "" {
			ccy = ln.priceCurrency
		}
	}
	return Order{
		id:          id,
		userID:      userID,
		customerID:  customerID,
		shipTo:      shipTo,
		shipMethod:  shipMethod,
		payMethod:   payMethod,
		items:       items,
		subtotalAmt: subtotal,
		totalAmt:    subtotal + shipMethod.Cost(),
		totalCcy:    ccy,
		status:      status,
		placedAt:    placedAt,
	}
}

func (o Order) ID() string                     { return o.id }
func (o Order) UserID() string                 { return o.userID }
func (o Order) CustomerID() string             { return o.customerID }
func (o Order) ShipTo() Address                { return o.shipTo }
func (o Order) ShippingMethod() ShippingMethod { return o.shipMethod }
func (o Order) PaymentMethod() PaymentMethod   { return o.payMethod }
func (o Order) Subtotal() int64                { return o.subtotalAmt }
func (o Order) SubtotalDisplay() string        { return money(o.subtotalAmt) }
func (o Order) ShippingCost() int64            { return o.shipMethod.Cost() }
func (o Order) ShippingCostDisplay() string    { return money(o.shipMethod.Cost()) }
func (o Order) Items() []Line         { return o.items }
func (o Order) Status() Status        { return o.status }
func (o Order) PlacedAt() time.Time   { return o.placedAt }
func (o Order) TotalAmount() int64    { return o.totalAmt }
func (o Order) TotalCurrency() string { return o.totalCcy }
func (o Order) TotalDisplay() string  { return money(o.totalAmt) }

// --- event-sourced write side ---

// PlaceOrder is the command that creates a new order from a cart snapshot
// and the customer's checkout choices. It emits an OrderPlaced event.
func PlaceOrder(id, userID, customerID string, shipTo Address, shipMethod ShippingMethod, payMethod PaymentMethod, lines []Line, at time.Time) (*Order, error) {
	if len(lines) == 0 {
		return nil, ErrCartEmpty
	}
	o := &Order{}
	o.raise(OrderPlaced{
		OrderID:    id,
		UserID:     userID,
		CustomerID: customerID,
		ShipTo:     shipTo,
		ShipMethod: shipMethod,
		PayMethod:  payMethod,
		Lines:      lines,
		At:         at,
	})
	return o, nil
}

// MarkPaid records a successful payment capture.
func (o *Order) MarkPaid(at time.Time) { o.raise(PaymentSucceeded{OrderID: o.id, At: at}) }

// MarkFailed records a declined payment.
func (o *Order) MarkFailed(reason string, at time.Time) {
	o.raise(PaymentFailed{OrderID: o.id, Reason: reason, At: at})
}

func (o *Order) raise(e Event) {
	o.apply(e)
	o.pendingEvents = append(o.pendingEvents, e)
}

// apply folds a single event into the aggregate's state. It is the only
// place order state changes, and it must stay free of side effects so it can
// replay history deterministically.
func (o *Order) apply(e Event) {
	switch ev := e.(type) {
	case OrderPlaced:
		o.id = ev.OrderID
		o.userID = ev.UserID
		o.customerID = ev.CustomerID
		o.shipTo = ev.ShipTo
		o.shipMethod = ev.ShipMethod
		o.payMethod = ev.PayMethod
		o.items = ev.Lines
		var subtotal int64
		var ccy string
		for _, ln := range ev.Lines {
			subtotal += ln.LineTotal()
			if ccy == "" {
				ccy = ln.priceCurrency
			}
		}
		o.subtotalAmt = subtotal
		o.totalAmt = subtotal + ev.ShipMethod.Cost()
		o.totalCcy = ccy
		o.status = StatusPending
		o.placedAt = ev.At
	case PaymentSucceeded:
		o.status = StatusPaid
	case PaymentFailed:
		o.status = StatusFailed
	}
	o.version++
}

// PendingEvents returns events raised but not yet persisted.
func (o *Order) PendingEvents() []Event { return o.pendingEvents }

// ClearPending drops the uncommitted events after they are persisted.
func (o *Order) ClearPending() { o.pendingEvents = nil }

// ExpectedVersion is the aggregate version before the pending events — i.e.
// the sequence the first pending event must follow in the store.
func (o *Order) ExpectedVersion() int { return o.version - len(o.pendingEvents) }

// RehydrateOrder rebuilds an order by folding its full event history.
func RehydrateOrder(events []Event) *Order {
	o := &Order{}
	for _, e := range events {
		o.apply(e)
	}
	o.pendingEvents = nil
	return o
}

// money formats an amount stored in minor units (e.g. cents) as "X.YY".
func money(amount int64) string {
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}
