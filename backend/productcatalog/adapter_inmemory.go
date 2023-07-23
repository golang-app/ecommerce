package productcatalog

import (
	"context"
)

type inMemory struct {
	products []Product
}

func NewInMemory() *inMemory {
	return &inMemory{}
}

func (im *inMemory) Add(ctx context.Context, p Product) error {
	im.products = append(im.products, p)

	return nil
}

func (im *inMemory) All(ctx context.Context) ([]Product, error) {
	return im.products, nil
}

func (im *inMemory) Find(ctx context.Context, id string) (Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return p, nil
		}
	}
	return Product{}, ErrProductNotFound
}

func (im *inMemory) Reserve(ctx context.Context, name string) error {
	return nil
}
