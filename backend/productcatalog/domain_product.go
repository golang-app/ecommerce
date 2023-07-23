package productcatalog

import (
	"errors"
	"regexp"
)

var ErrProductNotFound = errors.New("product not found")

type Price struct {
	amount   float64
	currency string
}

func NewPrice(amount float64, currency string) Price {
	return Price{
		amount:   amount,
		currency: currency,
	}
}

func (p Price) Amount() float64 {
	return p.amount
}

func (p Price) Currency() string {
	return p.currency
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
