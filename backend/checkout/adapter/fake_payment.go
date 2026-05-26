package adapter

import "context"

// FakePayment is a payment processor that always succeeds. It exists so the
// checkout flow has something to call without integrating a real provider.
// Swap with a real implementation (Stripe, Adyen, etc.) when you're ready.
type FakePayment struct{}

func NewFakePayment() FakePayment { return FakePayment{} }

func (FakePayment) Charge(_ context.Context, _ int64, _, _ string) error {
	return nil
}
