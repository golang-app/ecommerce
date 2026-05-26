package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidPaymentMethod = errors.New("invalid payment method")

// PaymentMethod is how the customer chose to pay. The actual charge is
// handled by the PaymentProcessor; this records the selection and drives
// whether card details are required at checkout.
type PaymentMethod struct {
	code        string
	label       string
	requiresCard bool
}

// availablePaymentMethods is the ordered catalogue offered at checkout.
var availablePaymentMethods = []PaymentMethod{
	{code: "card", label: "Credit / debit card", requiresCard: true},
	{code: "paypal", label: "PayPal", requiresCard: false},
	{code: "cod", label: "Cash on delivery", requiresCard: false},
}

// PaymentMethods returns the offered methods in display order.
func PaymentMethods() []PaymentMethod {
	out := make([]PaymentMethod, len(availablePaymentMethods))
	copy(out, availablePaymentMethods)
	return out
}

// PaymentMethodByCode resolves a method from the checkout catalogue.
func PaymentMethodByCode(code string) (PaymentMethod, error) {
	for _, m := range availablePaymentMethods {
		if m.code == code {
			return m, nil
		}
	}
	return PaymentMethod{}, fmt.Errorf("%w: %q", ErrInvalidPaymentMethod, code)
}

// RebuildPaymentMethod reconstructs a method snapshot from storage.
func RebuildPaymentMethod(code, label string) PaymentMethod {
	return PaymentMethod{
		code:        code,
		label:       label,
		requiresCard: code == "card",
	}
}

func (m PaymentMethod) Code() string       { return m.code }
func (m PaymentMethod) Label() string      { return m.label }
func (m PaymentMethod) RequiresCard() bool { return m.requiresCard }
func (m PaymentMethod) IsZero() bool       { return m.code == "" }
