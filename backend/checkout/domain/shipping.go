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
	carrier         string
	enabled         bool
}

// NewShippingMethod constructs a new ShippingMethod with all attributes.
func NewShippingMethod(code, label string, cost int64, requiresAddress bool, carrier string, enabled bool) ShippingMethod {
	return ShippingMethod{
		code:            code,
		label:           label,
		cost:            cost,
		requiresAddress: requiresAddress,
		carrier:         carrier,
		enabled:         enabled,
	}
}

// availableShippingMethods is the ordered catalogue offered at checkout.
var availableShippingMethods = []ShippingMethod{
	NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
	NewShippingMethod("pickup", "Personal pickup", 0, false, "Store Pickup", true),
	NewShippingMethod("courier", "Courier", 1500, true, "Express Courier", true),
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
		carrier:         "",
		enabled:         true,
	}
}

func (m ShippingMethod) Code() string          { return m.code }
func (m ShippingMethod) Label() string         { return m.label }
func (m ShippingMethod) Cost() int64           { return m.cost }
func (m ShippingMethod) CostDisplay() string   { return money(m.cost) }
func (m ShippingMethod) RequiresAddress() bool { return m.requiresAddress }
func (m ShippingMethod) Carrier() string       { return m.carrier }
func (m ShippingMethod) IsEnabled() bool       { return m.enabled }
func (m ShippingMethod) IsZero() bool          { return m.code == "" }
