package productcatalog

import (
	"context"
)

type InMemory struct {
	products []Product
}

func NewInMemory() *InMemory {
	return &InMemory{}
}

func (im *InMemory) Add(ctx context.Context, p Product) error {
	im.products = append(im.products, p)

	return nil
}

func (im *InMemory) All(ctx context.Context) ([]Product, error) {
	return im.products, nil
}

func (im *InMemory) Find(ctx context.Context, id string) (Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return p, nil
		}
	}
	return Product{}, ErrProductNotFound
}

func (im *InMemory) Reserve(ctx context.Context, name string) error {
	return nil
}
