package productcatalog

import "strings"

// OptionType is a product attribute the customer chooses, e.g. "Color" with
// values ["Red", "Blue"]. The order of values is the display order.
type OptionType struct {
	name   string
	values []string
}

func NewOptionType(name string, values []string) OptionType {
	return OptionType{name: name, values: values}
}

func (o OptionType) Name() string     { return o.name }
func (o OptionType) Values() []string { return o.values }

// Variant is a concrete, purchasable version of a product — a specific
// combination of option values with its own price. A product with no options
// has a single default variant whose options map is empty.
type Variant struct {
	id      string
	sku     string
	options map[string]string // e.g. {"Color":"Red","Size":"L"}
	price   Price
}

func NewVariant(id, sku string, options map[string]string, price Price) Variant {
	if options == nil {
		options = map[string]string{}
	}
	return Variant{id: id, sku: sku, options: options, price: price}
}

func (v Variant) ID() string                 { return v.id }
func (v Variant) SKU() string                { return v.sku }
func (v Variant) Options() map[string]string { return v.options }
func (v Variant) Price() Price               { return v.price }
func (v Variant) IsZero() bool               { return v.id == "" }

// Label builds a human label from the variant's option values, in the
// product's option-type order (e.g. "Red / L"). Empty for a default variant.
func (v Variant) Label(optionTypes []OptionType) string {
	parts := make([]string, 0, len(optionTypes))
	for _, ot := range optionTypes {
		if val, ok := v.options[ot.name]; ok && val != "" {
			parts = append(parts, val)
		}
	}
	return strings.Join(parts, " / ")
}
