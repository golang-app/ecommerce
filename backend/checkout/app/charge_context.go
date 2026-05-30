package app

import "context"

// orderIDKey and idempotencyKeyKey are the package-private context
// keys threaded between Place and the PaymentProcessor implementation.
// The unexported struct-type pattern is the standard way to give
// context.WithValue keys namespace safety: two different packages
// using the same name produce distinct types and therefore distinct
// keys.
type orderIDKey struct{}

type idempotencyKeyKey struct{}

// WithChargeContext stashes the order id and per-attempt idempotency
// key on the context Place hands down to PaymentProcessor.Charge. The
// payments-backed adapter reads them through ChargeContextValues to
// decide what to send to the payments Service.
//
// The PaymentProcessor port intentionally still takes the same
// (ctx, amount, currency, cardNumber) signature — widening it would
// force every other implementation (a future Adyen, a test fake) to
// adopt the same parameter list just to demonstrate this pattern.
// Context threading keeps the port narrow and gives the producer /
// consumer a typed channel for the extras.
func WithChargeContext(ctx context.Context, orderID, idempotencyKey string) context.Context {
	ctx = context.WithValue(ctx, orderIDKey{}, orderID)
	ctx = context.WithValue(ctx, idempotencyKeyKey{}, idempotencyKey)
	return ctx
}

// ChargeContextValues is the reader side of WithChargeContext. An
// empty string is returned for any key that was not set, which lets
// the historical FakePayment path (no producer call) keep working
// without a special "is this context populated?" check.
func ChargeContextValues(ctx context.Context) (orderID, idempotencyKey string) {
	if v, ok := ctx.Value(orderIDKey{}).(string); ok {
		orderID = v
	}
	if v, ok := ctx.Value(idempotencyKeyKey{}).(string); ok {
		idempotencyKey = v
	}
	return orderID, idempotencyKey
}
