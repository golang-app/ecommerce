package app

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type Product struct {
	storage Storage
}

type Storage interface {
	All(ctx context.Context) ([]domain.Product, error)
	Add(ctx context.Context, p domain.Product) error
	Find(ctx context.Context, id string) (domain.Product, error)
}

func NewProduct(s Storage) Product {
	return Product{storage: s}
}

func (p Product) AllProducts(ctx context.Context) ([]domain.Product, error) {
	return p.storage.All(ctx)
}

func (p Product) Find(ctx context.Context, id string) (domain.Product, error) {
	return p.storage.Find(ctx, id)
}
