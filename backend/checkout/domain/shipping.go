package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidShippingMethod = errors.New("invalid shipping method")

// ShippingMethod is the delivery option chosen at checkout. Cost is in minor
// units (e.g. cents). Personal pickup is free and needs no address.
type ShippingMethod struct {
	code            string
	label           string
	cost            int64
	requiresAddress bool
}

// availableShippingMethods is the ordered catalogue offered at checkout.
var availableShippingMethods = []ShippingMethod{
	{code: "flat", label: "Flat rate", cost: 500, requiresAddress: true},
	{code: "pickup", label: "Personal pickup", cost: 0, requiresAddress: false},
	{code: "courier", label: "Courier", cost: 1500, requiresAddress: true},
}

// ShippingMethods returns the offered methods in display order.
func ShippingMethods() []ShippingMethod {
	out := make([]ShippingMethod, len(availableShippingMethods))
	copy(out, availableShippingMethods)
	return out
}

// ShippingMethodByCode resolves a method from the checkout catalogue.
func ShippingMethodByCode(code string) (ShippingMethod, error) {
	for _, m := range availableShippingMethods {
		if m.code == code {
			return m, nil
		}
	}
	return ShippingMethod{}, fmt.Errorf("%w: %q", ErrInvalidShippingMethod, code)
}

// RebuildShippingMethod reconstructs a method snapshot from storage. The
// label and cost are taken from the stored order (so historical orders keep
// the price they were charged even if the catalogue later changes).
func RebuildShippingMethod(code, label string, cost int64) ShippingMethod {
	return ShippingMethod{
		code:            code,
		label:           label,
		cost:            cost,
		requiresAddress: code != "pickup",
	}
}

func (m ShippingMethod) Code() string          { return m.code }
func (m ShippingMethod) Label() string         { return m.label }
func (m ShippingMethod) Cost() int64           { return m.cost }
func (m ShippingMethod) CostDisplay() string   { return money(m.cost) }
func (m ShippingMethod) RequiresAddress() bool { return m.requiresAddress }
func (m ShippingMethod) IsZero() bool          { return m.code == "" }
