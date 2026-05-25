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

// Product is an entity that represents a single product visable in the product catalog
type Product struct {
	id          ProductID
	name        string
	description string
	price       Price
	thumbnail   string
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
