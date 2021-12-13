package adapter

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type InMemory struct {
	products []domain.Product
}

func NewInMemory() *InMemory {
	return &InMemory{}
}

func (im *InMemory) Add(ctx context.Context, p domain.Product) error {
	im.products = append(im.products, p)

	return nil
}

func (im *InMemory) All(ctx context.Context) ([]domain.Product, error) {
	return im.products, nil
}

func (im *InMemory) Find(ctx context.Context, id string) (domain.Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return p, nil
		}
	}
	return domain.Product{}, domain.ErrProductNotFound
}

func (im *InMemory) Reserve(ctx context.Context, name string) error {
	return nil
}
