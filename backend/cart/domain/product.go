package domain

import (
	"errors"
)

var ErrProductNotFound = errors.New("product not found")

type Product struct {
	id    string
	name  string
	price price
}

func NewProduct(id string, name string, amount float64, currency string) Product {
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

type price struct {
	amount   float64
	currency string
}

func NewPrice(amount float64, currency string) price {
	return price{amount: amount, currency: currency}
}

func (p price) Amount() float64 {
	return p.amount
}

func (p price) Multiple(d int) price {
	p.amount *= float64(d)
	return p
}

func (p price) Add(p2 price) price {
	p.amount += p2.amount
	return p
}

func (p price) Currency() string {
	return p.currency
}
