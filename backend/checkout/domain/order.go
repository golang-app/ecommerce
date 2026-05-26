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
	items       []Line
	subtotalAmt int64
	totalAmt    int64
	totalCcy    string
	status      Status
	placedAt    time.Time
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
func NewOrder(id, userID, customerID string, shipTo Address, shipMethod ShippingMethod, items []Line, status Status, placedAt time.Time) Order {
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

// WithStatus returns a copy of the order with a different status. Used by
// the checkout service to flip pending -> paid / failed after a payment
// attempt.
func (o Order) WithStatus(s Status) Order {
	o.status = s
	return o
}

// money formats an amount stored in minor units (e.g. cents) as "X.YY".
func money(amount int64) string {
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}
