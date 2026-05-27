// Package query holds the read side of the checkout context (CQRS): flat read
// models projected from the order tables, deliberately separate from the
// event-sourced write aggregate (checkout/domain.Order). They reuse the
// context's value objects but are never used to issue commands.
package query

import (
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

func money(amount int64) string {
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}

// OrderSummary is the read model for order lists (account orders, overview,
// admin order list).
type OrderSummary struct {
	customerID string
	id         string
	status     domain.Status
	placedAt   time.Time
	itemCount  int
	total      int64
	currency   string
}

func NewOrderSummary(customerID, id string, status domain.Status, placedAt time.Time, itemCount int, total int64, currency string) OrderSummary {
	return OrderSummary{customerID: customerID, id: id, status: status, placedAt: placedAt, itemCount: itemCount, total: total, currency: currency}
}

func (s OrderSummary) CustomerID() string    { return s.customerID }
func (s OrderSummary) ID() string            { return s.id }
func (s OrderSummary) Status() domain.Status { return s.status }
func (s OrderSummary) PlacedAt() time.Time   { return s.placedAt }
func (s OrderSummary) ItemCount() int        { return s.itemCount }
func (s OrderSummary) TotalDisplay() string  { return money(s.total) }
func (s OrderSummary) TotalCurrency() string { return s.currency }

// OrderView is the read model for the order detail page.
type OrderView struct {
	id         string
	customerID string
	status     domain.Status
	placedAt   time.Time
	items      []domain.Line
	shipTo     domain.Address
	shipMethod domain.ShippingMethod
	payMethod  domain.PaymentMethod
	subtotal   int64
	total      int64
	currency   string
}

func NewOrderView(
	id, customerID string,
	status domain.Status,
	placedAt time.Time,
	items []domain.Line,
	shipTo domain.Address,
	shipMethod domain.ShippingMethod,
	payMethod domain.PaymentMethod,
	subtotal, total int64,
	currency string,
) OrderView {
	return OrderView{
		id: id, customerID: customerID, status: status, placedAt: placedAt,
		items: items, shipTo: shipTo, shipMethod: shipMethod, payMethod: payMethod,
		subtotal: subtotal, total: total, currency: currency,
	}
}

func (v OrderView) ID() string                            { return v.id }
func (v OrderView) CustomerID() string                    { return v.customerID }
func (v OrderView) Status() domain.Status                 { return v.status }
func (v OrderView) PlacedAt() time.Time                   { return v.placedAt }
func (v OrderView) Items() []domain.Line                  { return v.items }
func (v OrderView) ShipTo() domain.Address                { return v.shipTo }
func (v OrderView) ShippingMethod() domain.ShippingMethod { return v.shipMethod }
func (v OrderView) PaymentMethod() domain.PaymentMethod   { return v.payMethod }
func (v OrderView) SubtotalDisplay() string               { return money(v.subtotal) }
func (v OrderView) ShippingCostDisplay() string           { return money(v.shipMethod.Cost()) }
func (v OrderView) TotalDisplay() string                  { return money(v.total) }
func (v OrderView) TotalCurrency() string                 { return v.currency }
