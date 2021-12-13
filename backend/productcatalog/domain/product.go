package domain

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

var ErrProductNotFound = errors.New("product not found")

type Price struct {
	amount   float32
	currency string
}

func NewPrice(amount float32, currency string) Price {
	return Price{
		amount:   amount,
		currency: currency,
	}
}

func (p Price) Amount() float32 {
	return p.amount
}

func (p Price) Currency() string {
	return p.currency
}

// Product is an entity that represents a single product visable in the product catalog
type Product struct {
	id          productID
	name        string
	description string
	price       Price
	thumbnail   string
}

var productIDReg = regexp.MustCompile(`[\w\d\-]+`)

type productID string

func NewProductId(id string) (productID, error) {
	if !productIDReg.MatchString(id) {
		return productID(""), errors.New("the ID doesn't match")
	}

	return productID(id), nil
}

type productBuilder struct {
	reservation ProductIdReservation
}

func NewProductBuilder(reservation ProductIdReservation) productBuilder {
	return productBuilder{reservation: reservation}
}

func (pb productBuilder) Build(ctx context.Context, name, description string, price Price, thumbnail string) (Product, error) {
	id, err := pb.reservation.Reserve(ctx, name)
	if err != nil {
		return Product{}, fmt.Errorf("cannot build the product: %w", err)
	}

	return NewProduct(id, name, description, price, thumbnail)
}

func NewProduct(id productID, name, description string, price Price, thumbnail string) (Product, error) {
	if name == "" {
		return Product{}, errors.New("the name cannot be empty")
	}

	if description == "" {
		return Product{}, errors.New("the description cannot be empty")
	}

	return Product{
		id:          id,
		name:        name,
		price:       price,
		description: description,
		thumbnail:   thumbnail,
	}, nil
}

func (p Product) ID() productID {
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
