// OrderFactory front-loads construction-time validation for the Order
// aggregate, separating "is this a valid order to create?" (factory's job)
// from "the order has been placed" (the PlaceOrder command's job).
//
// The two-step separation matters because:
//   - failure modes for construction are richer than 'something went wrong'
//     (cart empty / address missing / channel unset have different remedies)
//   - the aggregate's command methods (MarkPaid, Cancel, etc.) can assume
//     a fully-valid aggregate without re-checking construction invariants
//   - tests can exercise construction in isolation from event-raising
//
// In DDD terms: this is an Aggregate Factory (Evans 6.4).
package domain

import (
	"errors"
	"time"
)

// Construction-time sentinels surfaced by OrderFactory.FromCart. Each one
// names a specific, actionable failure so the orchestrator (and the HTTP
// handler above it) can show the customer the right remediation rather
// than a generic "something went wrong".
var (
	// ErrAddressRequired is returned when the chosen shipping method
	// requires a destination address (RequiresAddress()) but the supplied
	// Address is the zero value.
	ErrAddressRequired = errors.New("shipping address is required for the chosen shipping method")
	// ErrChannelRequired is returned when the caller did not specify the
	// sales channel ("web", "ios", "api", ...). The OrderPlaced v2 schema
	// keeps Channel non-empty; rejecting it here prevents an empty value
	// from leaking into the event log.
	ErrChannelRequired = errors.New("sales channel is required")
	// ErrPaymentMethodRequired is returned when the supplied PaymentMethod
	// has no code (the zero value). It is a belt-and-braces guard — the
	// HTTP handler already resolves the method via PaymentMethodByCode —
	// but it keeps the domain self-protecting.
	ErrPaymentMethodRequired = errors.New("payment method is required")
)

// OrderFactory builds Order aggregates from a checkout snapshot, enforcing
// the construction invariants that the bare PlaceOrder command no longer
// re-checks. It is currently stateless; an id generator or clock can be
// added as fields here without disturbing call sites that already use the
// FromCart method.
type OrderFactory struct{}

// NewOrderFactory returns a ready-to-use OrderFactory. The factory is
// value-typed and stateless today, so callers may construct one inline
// (`domain.OrderFactory{}.FromCart(...)`); the constructor exists so we
// have a single place to wire dependencies if any are added later.
func NewOrderFactory() OrderFactory { return OrderFactory{} }

// FromCart validates the construction-time invariants of an Order and
// delegates the actual event-raising to PlaceOrder. The Quote carries the
// already-computed Tax / ShippingCost / DiscountAmount — the orchestrator
// derives those upstream via domain.PriceQuote and hands them in as a
// single value object so this signature stays compact.
//
// Returns one of: ErrCartEmpty, ErrAddressRequired, ErrChannelRequired,
// ErrPaymentMethodRequired (each named so the orchestrator can map it to
// the appropriate remediation), or nil + the placed *Order on success.
func (OrderFactory) FromCart(
	id, userID, customerID string,
	shipTo Address,
	shipMethod ShippingMethod,
	payMethod PaymentMethod,
	lines []Line,
	quote Quote,
	discountCode string,
	channel string,
	at time.Time,
) (*Order, error) {
	if len(lines) == 0 {
		return nil, ErrCartEmpty
	}
	if shipMethod.RequiresAddress() && shipTo.IsZero() {
		return nil, ErrAddressRequired
	}
	if channel == "" {
		return nil, ErrChannelRequired
	}
	if payMethod.Code() == "" {
		return nil, ErrPaymentMethodRequired
	}
	// Construction is valid — delegate to the aggregate command. PlaceOrder
	// no longer re-checks the empty-cart guard (that is the factory's job);
	// it trusts the inputs and focuses on raising OrderPlaced.
	return PlaceOrder(
		id, userID, customerID,
		shipTo, shipMethod, payMethod,
		lines,
		quote.Tax, quote.ShippingCost,
		discountCode, quote.DiscountAmount,
		channel, at,
	)
}
