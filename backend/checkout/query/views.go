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

// TotalAmount is the row's grand total in minor units of the order's
// stored currency. Lists in the storefront pass this into the
// currency-aware `money` template helper so the displayed total
// follows the customer's chosen currency.
func (s OrderSummary) TotalAmount() int64    { return s.total }
func (s OrderSummary) TotalDisplay() string  { return money(s.total) }
func (s OrderSummary) TotalCurrency() string { return s.currency }

// OrderView is the read model for the order detail page.
type OrderView struct {
	id             string
	customerID     string
	status         domain.Status
	placedAt       time.Time
	items          []domain.Line
	shipTo         domain.Address
	shipMethod     domain.ShippingMethod
	payMethod      domain.PaymentMethod
	subtotal       int64
	tax            int64
	shipCost       int64
	total          int64
	currency       string
	carrier        string
	trackingCode   string
	discountCode   string
	discountAmount int64
}

func NewOrderView(
	id, customerID string,
	status domain.Status,
	placedAt time.Time,
	items []domain.Line,
	shipTo domain.Address,
	shipMethod domain.ShippingMethod,
	payMethod domain.PaymentMethod,
	subtotal, tax, shipCost, total int64,
	currency string,
	carrier, trackingCode string,
	discountCode string,
	discountAmount int64,
) OrderView {
	return OrderView{
		id: id, customerID: customerID, status: status, placedAt: placedAt,
		items: items, shipTo: shipTo, shipMethod: shipMethod, payMethod: payMethod,
		subtotal: subtotal, tax: tax, shipCost: shipCost, total: total, currency: currency,
		carrier: carrier, trackingCode: trackingCode,
		discountCode: discountCode, discountAmount: discountAmount,
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

// Subtotal returns the order's subtotal in minor units of the order's
// stored currency. Exposed as an int64 so the storefront templates can
// hand it to the currency-aware `money` FuncMap helper for conversion.
func (v OrderView) Subtotal() int64             { return v.subtotal }
func (v OrderView) SubtotalDisplay() string     { return money(v.subtotal) }
func (v OrderView) ShippingCost() int64         { return v.shipCost }
func (v OrderView) ShippingCostDisplay() string { return money(v.shipCost) }
func (v OrderView) TaxAmount() int64            { return v.tax }
func (v OrderView) TaxDisplay() string          { return money(v.tax) }

// TotalAmount returns the order's grand total in minor units of the
// order's stored currency. As with Subtotal, the int64 view lets the
// storefront templates convert via the `money` helper while the
// existing Display() string still drives any legacy / email render.
func (v OrderView) TotalAmount() int64    { return v.total }
func (v OrderView) TotalDisplay() string  { return money(v.total) }
func (v OrderView) TotalCurrency() string { return v.currency }
func (v OrderView) Carrier() string       { return v.carrier }
func (v OrderView) TrackingCode() string  { return v.trackingCode }

// DiscountCode is the literal promo code applied at place time (empty when
// none was used).
func (v OrderView) DiscountCode() string { return v.discountCode }

// DiscountAmount is the discount in minor units that was subtracted from
// the subtotal before tax (0 when none).
func (v OrderView) DiscountAmount() int64 { return v.discountAmount }

// DiscountDisplay renders the discount amount as the templates' totals
// row shows it (e.g. "5.00"). Templates add the negative sign / the code
// label themselves so the conditional formatting stays in the view.
func (v OrderView) DiscountDisplay() string { return money(v.discountAmount) }

// FreeShipping reports whether the order's shipping was zero AND a
// discount code was applied — useful in templates to label the discount
// row as "free shipping (CODE)" instead of a money amount.
func (v OrderView) FreeShipping() bool {
	return v.discountCode != "" && v.shipCost == 0 && v.discountAmount == 0
}
