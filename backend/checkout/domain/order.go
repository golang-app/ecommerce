package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/sharedkernel"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusPaid      Status = "paid"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusShipped   Status = "shipped"
	StatusDelivered Status = "delivered"
	StatusRefunded  Status = "refunded"
)

var (
	ErrOrderNotFound       = errors.New("order not found")
	ErrCartEmpty           = errors.New("cannot place order from an empty cart")
	ErrOrderNotCancellable = errors.New("order cannot be cancelled")
	ErrOrderNotShippable   = errors.New("order cannot be shipped")
	ErrOrderNotDeliverable = errors.New("order cannot be delivered")
	ErrOrderNotRefundable  = errors.New("order cannot be refunded")
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
	id           string
	userID       string
	customerID   string
	shipTo       Address
	shipMethod   ShippingMethod
	payMethod    PaymentMethod
	items        []Line
	subtotalAmt  int64
	taxAmt       int64
	shipCostAmt  int64
	discountCode string
	discountAmt  int64
	totalAmt     int64
	totalCcy     string
	status       Status
	placedAt     time.Time
	carrier      string
	trackingCode string
	channel      string

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
		shipCostAmt: shipMethod.Cost(),
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
func (o Order) ShippingCost() int64            { return o.shipCostAmt }
func (o Order) ShippingCostDisplay() string    { return money(o.shipCostAmt) }
func (o Order) TaxAmount() int64               { return o.taxAmt }
func (o Order) TaxDisplay() string             { return money(o.taxAmt) }
func (o Order) Items() []Line         { return o.items }
func (o Order) Status() Status        { return o.status }
func (o Order) PlacedAt() time.Time   { return o.placedAt }
func (o Order) TotalAmount() int64    { return o.totalAmt }
func (o Order) TotalCurrency() string { return o.totalCcy }
func (o Order) TotalDisplay() string  { return money(o.totalAmt) }
func (o Order) Carrier() string       { return o.carrier }
func (o Order) TrackingCode() string  { return o.trackingCode }

// Channel returns the sales channel the order was placed through (e.g. "web",
// "ios", "api"). v1 OrderPlaced events lacked this field; the codec's
// upcaster fills "unknown" on load so historical orders read back with a
// stable, explicit value rather than the zero string.
func (o Order) Channel() string { return o.channel }

// DiscountCode returns the literal promo code applied at place time
// (empty when no code was used). The order is event-sourced so the
// historical value is preserved across replays.
func (o Order) DiscountCode() string { return o.discountCode }

// DiscountAmount returns the resolved discount in minor units that was
// subtracted from the subtotal before tax. For free-shipping codes this
// is 0 — the saving is reflected in ShippingCost instead.
func (o Order) DiscountAmount() int64 { return o.discountAmt }

// DiscountDisplay renders the discount amount for the totals row in the
// templates (negative sign included).
func (o Order) DiscountDisplay() string { return money(o.discountAmt) }

// --- Shared-kernel Money accessors (additive) ---
//
// These getters return the same numbers as the int64 accessors above, wrapped
// in the shared-kernel Money value object. They live ALONGSIDE the existing
// accessors so callers that already speak Money (or want to) can pick them
// up without breaking the int64-shaped reader paths (storage, templates,
// CQRS projections) that still pair amount+currency at the boundary. See
// internal/sharedkernel/README.md for the migration plan.

// TotalMoney returns the order's grand total as a Money value.
func (o Order) TotalMoney() sharedkernel.Money {
	return sharedkernel.MustNewMoney(o.totalAmt, sharedkernel.Currency(o.totalCcy))
}

// SubtotalMoney returns the pre-discount, pre-tax, pre-shipping subtotal.
func (o Order) SubtotalMoney() sharedkernel.Money {
	return sharedkernel.MustNewMoney(o.subtotalAmt, sharedkernel.Currency(o.totalCcy))
}

// TaxMoney returns the tax charged on the discounted subtotal.
func (o Order) TaxMoney() sharedkernel.Money {
	return sharedkernel.MustNewMoney(o.taxAmt, sharedkernel.Currency(o.totalCcy))
}

// ShippingCostMoney returns the effective shipping cost (0 when free
// shipping applied).
func (o Order) ShippingCostMoney() sharedkernel.Money {
	return sharedkernel.MustNewMoney(o.shipCostAmt, sharedkernel.Currency(o.totalCcy))
}

// DiscountMoney returns the resolved discount subtracted from the subtotal
// before tax (zero when no code, or when the code only zeroed shipping).
func (o Order) DiscountMoney() sharedkernel.Money {
	return sharedkernel.MustNewMoney(o.discountAmt, sharedkernel.Currency(o.totalCcy))
}

// --- event-sourced write side ---

// PlaceOrder is the command that creates a new order from a cart snapshot
// and the customer's checkout choices. It emits an OrderPlaced event.
//
// tax is the tax amount in minor units already computed by the caller
// (CheckoutService) from the configured rate AFTER the promo-code discount
// has been subtracted from the subtotal. effectiveShipping is the shipping
// price actually charged — 0 when a configured free-shipping threshold
// knocked it down, 0 when a free-shipping promo code was applied, or the
// catalogue method's Cost() otherwise.
//
// discountCode/discountAmount carry the resolved promo code (empty / 0 when
// none was used). The event-sourced aggregate keeps these so replaying
// history reproduces the original totals.
//
// channel records the sales channel ("web", "ios", "api", ...) the order was
// placed through. It became part of the OrderPlaced v2 schema; callers
// today always pass "web" (the only producer) but the parameter is wired
// through so iOS/API entry points can light it up without a second
// schema migration.
func PlaceOrder(id, userID, customerID string, shipTo Address, shipMethod ShippingMethod, payMethod PaymentMethod, lines []Line, tax, effectiveShipping int64, discountCode string, discountAmount int64, channel string, at time.Time) (*Order, error) {
	if len(lines) == 0 {
		return nil, ErrCartEmpty
	}
	o := &Order{}
	o.raise(OrderPlaced{
		OrderID:        id,
		UserID:         userID,
		CustomerID:     customerID,
		ShipTo:         shipTo,
		ShipMethod:     shipMethod,
		PayMethod:      payMethod,
		Lines:          lines,
		Tax:            tax,
		ShippingCost:   effectiveShipping,
		DiscountCode:   discountCode,
		DiscountAmount: discountAmount,
		Channel:        channel,
		At:             at,
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
		o.taxAmt = ev.Tax
		// Historical events written before the tax/shipping fields existed
		// will have ShippingCost == 0 and Tax == 0; fall back to the method
		// cost so those orders still display their original total.
		if ev.ShippingCost == 0 && ev.Tax == 0 {
			o.shipCostAmt = ev.ShipMethod.Cost()
		} else {
			o.shipCostAmt = ev.ShippingCost
		}
		// Promo code is replayed verbatim from the event so historical
		// orders preserve the math: the discount was subtracted from the
		// subtotal BEFORE tax, so the total is computed as
		//   (subtotal - discount) + tax + effective_shipping
		// — see checkout/app.Place for the live computation that
		// produced these fields.
		o.discountCode = ev.DiscountCode
		o.discountAmt = ev.DiscountAmount
		o.totalAmt = subtotal - o.discountAmt + o.taxAmt + o.shipCostAmt
		o.totalCcy = ccy
		o.status = StatusPending
		o.placedAt = ev.At
		// Channel is the v2 OrderPlaced field; v1 payloads are upcast to
		// "unknown" by the codec before they reach apply, so this is always
		// a meaningful string.
		o.channel = ev.Channel
	case PaymentSucceeded:
		o.status = StatusPaid
	case PaymentFailed:
		o.status = StatusFailed
	case OrderCancelled:
		o.status = StatusCancelled
	case OrderShipped:
		o.status = StatusShipped
		o.carrier = ev.Carrier
		o.trackingCode = ev.TrackingCode
	case OrderDelivered:
		o.status = StatusDelivered
	case OrderRefunded:
		o.status = StatusRefunded
	}
	o.version++
}

// Cancel cancels a paid order. Only paid orders can be cancelled — pending,
// failed and already-cancelled orders are rejected with ErrOrderNotCancellable.
func (o *Order) Cancel(reason string, at time.Time) error {
	if o.status != StatusPaid {
		return ErrOrderNotCancellable
	}
	o.raise(OrderCancelled{OrderID: o.id, Reason: reason, At: at})
	return nil
}

// MarkShipped records that the warehouse dispatched a paid order. Carrier
// and trackingCode are optional metadata (empty strings are allowed). The
// transition is only permitted from StatusPaid.
func (o *Order) MarkShipped(carrier, trackingCode string, at time.Time) error {
	if o.status != StatusPaid {
		return ErrOrderNotShippable
	}
	o.raise(OrderShipped{OrderID: o.id, Carrier: carrier, TrackingCode: trackingCode, At: at})
	return nil
}

// MarkDelivered records that a shipped order reached the customer. Only
// shipped orders can be marked delivered.
func (o *Order) MarkDelivered(at time.Time) error {
	if o.status != StatusShipped {
		return ErrOrderNotDeliverable
	}
	o.raise(OrderDelivered{OrderID: o.id, At: at})
	return nil
}

// Refund records that an admin reversed the charge on the order; refunds are
// allowed for paid, shipped or delivered orders (returning customers get
// their money back regardless of where the shipment is).
func (o *Order) Refund(reason string, at time.Time) error {
	switch o.status {
	case StatusPaid, StatusShipped, StatusDelivered:
		// allowed
	default:
		return ErrOrderNotRefundable
	}
	o.raise(OrderRefunded{OrderID: o.id, Reason: reason, At: at})
	return nil
}

// PendingEvents returns events raised but not yet persisted.
func (o *Order) PendingEvents() []Event { return o.pendingEvents }

// ClearPending drops the uncommitted events after they are persisted.
func (o *Order) ClearPending() { o.pendingEvents = nil }

// ExpectedVersion is the aggregate version before the pending events — i.e.
// the sequence the first pending event must follow in the store.
func (o *Order) ExpectedVersion() int { return o.version - len(o.pendingEvents) }

// Version returns the aggregate's current sequence number — i.e. the
// sequence of the last event that has been applied (events committed +
// events still pending). The adapter uses it to decide when to persist a
// snapshot.
func (o *Order) Version() int { return o.version }

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
