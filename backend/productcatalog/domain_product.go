package productcatalog

import (
	"errors"
	"fmt"
	"regexp"
)

var ErrProductNotFound = errors.New("product not found")

// Currency is an ISO 4217 three-letter currency code.
type Currency string

var currencyReg = regexp.MustCompile(`^[A-Z]{3}$`)

func NewCurrency(code string) (Currency, error) {
	if !currencyReg.MatchString(code) {
		return "", fmt.Errorf("invalid currency code %q: must be three uppercase letters (ISO 4217)", code)
	}
	return Currency(code), nil
}

func MustNewCurrency(code string) Currency {
	c, err := NewCurrency(code)
	if err != nil {
		panic(err)
	}
	return c
}

func (c Currency) String() string { return string(c) }

// Price holds an amount in minor currency units (e.g. cents for USD) and a currency.
type Price struct {
	amount   int64
	currency Currency
}

func NewPrice(amount int64, currency Currency) (Price, error) {
	if amount < 0 {
		return Price{}, fmt.Errorf("price amount cannot be negative: %d", amount)
	}
	if currency == "" {
		return Price{}, errors.New("price currency cannot be empty")
	}
	return Price{amount: amount, currency: currency}, nil
}

func MustNewPrice(amount int64, currency Currency) Price {
	p, err := NewPrice(amount, currency)
	if err != nil {
		panic(err)
	}
	return p
}

// Amount returns the price amount in minor units (e.g. cents).
func (p Price) Amount() int64 {
	return p.amount
}

func (p Price) Currency() Currency {
	return p.currency
}

func (p Price) Equals(other Price) bool {
	return p.amount == other.amount && p.currency == other.currency
}

// Display formats the amount as a decimal string in major units (e.g. "2.34").
func (p Price) Display() string {
	return fmt.Sprintf("%d.%02d", p.amount/100, p.amount%100)
}

// Product is an entity that represents a single product visable in the product catalog.
// Its purchasable units are its variants; price is the product's base/display
// price (the price the catalogue was seeded with).
type Product struct {
	id          ProductID
	name        string
	description string
	price       Price
	thumbnail   string
	optionTypes []OptionType
	variants    []Variant
}

var emptyProduct = Product{}

var productIDReg = regexp.MustCompile(`[\w\d\-]+`)

type ProductID string

func NewProductId(id string) (ProductID, error) {
	if !productIDReg.MatchString(id) {
		return ProductID(""), errors.New("the ID doesn't match")
	}

	return ProductID(id), nil
}

func NewProduct(id ProductID, name, description string, price Price, thumbnail string) (Product, error) {
	if name == "" {
		return emptyProduct, errors.New("the name cannot be empty")
	}

	if description == "" {
		return emptyProduct, errors.New("the description cannot be empty")
	}

	return Product{
		id:          id,
		name:        name,
		price:       price,
		description: description,
		thumbnail:   thumbnail,
	}, nil
}

func (p Product) ID() ProductID {
	return p.id
}

func (p Product) Name() string {
	return p.name
}

func (p Product) Description() string {
	return p.description
}

func (p Product) Price() Price {
	return p.price
}

func (p Product) Thumbnail() string {
	return p.thumbnail
}

// WithCatalog returns a copy of the product with its option types and
// variants attached (used by the storage layer after loading them).
func (p Product) WithCatalog(optionTypes []OptionType, variants []Variant) Product {
	p.optionTypes = optionTypes
	p.variants = variants
	return p
}

func (p Product) OptionTypes() []OptionType { return p.optionTypes }
func (p Product) Variants() []Variant       { return p.variants }
func (p Product) HasOptions() bool          { return len(p.optionTypes) > 0 }

// Variant returns the variant with the given id, if it belongs to this product.
func (p Product) Variant(id string) (Variant, bool) {
	for _, v := range p.variants {
		if v.id == id {
			return v, true
		}
	}
	return Variant{}, false
}

// DefaultVariant is the first variant (used when a product has no options).
func (p Product) DefaultVariant() Variant {
	if len(p.variants) > 0 {
		return p.variants[0]
	}
	return Variant{}
}

// PriceFrom is the lowest variant price, for "from $X" display. Falls back to
// the product base price when there are no variants.
func (p Product) PriceFrom() Price {
	if len(p.variants) == 0 {
		return p.price
	}
	min := p.variants[0].price
	for _, v := range p.variants[1:] {
		if v.price.Amount() < min.Amount() {
			min = v.price
		}
	}
	return min
}

// HasPriceRange reports whether variants span more than one price (so the UI
// can show "from $X" rather than a single price).
func (p Product) HasPriceRange() bool {
	if len(p.variants) < 2 {
		return false
	}
	first := p.variants[0].price.Amount()
	for _, v := range p.variants[1:] {
		if v.price.Amount() != first {
			return true
		}
	}
	return false
}

// ResolveVariant finds the variant matching the selected option values
// (keyed by option-type name).
func (p Product) ResolveVariant(selected map[string]string) (Variant, bool) {
	for _, v := range p.variants {
		match := true
		for _, ot := range p.optionTypes {
			if v.options[ot.name] != selected[ot.name] {
				match = false
				break
			}
		}
		if match {
			return v, true
		}
	}
	return Variant{}, false
}
