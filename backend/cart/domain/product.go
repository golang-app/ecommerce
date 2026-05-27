package domain

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrProductNotFound  = errors.New("product not found")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrOutOfStock       = errors.New("variant is out of stock")
)

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

type Product struct {
	id    string
	name  string
	price price
}

func NewProduct(id string, name string, amount int64, currency Currency) Product {
	return Product{id: id, name: name, price: price{amount: amount, currency: currency}}
}

func (p Product) ID() string {
	return p.id
}

func (p Product) Price() price {
	return p.price
}

func (p Product) Name() string {
	return p.name
}

// price holds an amount in minor currency units (e.g. cents).
type price struct {
	amount   int64
	currency Currency
}

func NewPrice(amount int64, currency Currency) (price, error) {
	if amount < 0 {
		return price{}, fmt.Errorf("price amount cannot be negative: %d", amount)
	}
	if currency == "" {
		return price{}, errors.New("price currency cannot be empty")
	}
	return price{amount: amount, currency: currency}, nil
}

func MustNewPrice(amount int64, currency Currency) price {
	p, err := NewPrice(amount, currency)
	if err != nil {
		panic(err)
	}
	return p
}

func (p price) Amount() int64 {
	return p.amount
}

func (p price) Currency() Currency {
	return p.currency
}

func (p price) Equals(other price) bool {
	return p.amount == other.amount && p.currency == other.currency
}

// Display formats the amount as a decimal string in major units (e.g. "2.34").
func (p price) Display() string {
	return fmt.Sprintf("%d.%02d", p.amount/100, p.amount%100)
}

func (p price) Multiple(d int) price {
	p.amount *= int64(d)
	return p
}

func (p price) Add(p2 price) (price, error) {
	if p.currency != p2.currency {
		return price{}, fmt.Errorf("%w: %s + %s", ErrCurrencyMismatch, p.currency, p2.currency)
	}
	p.amount += p2.amount
	return p, nil
}
