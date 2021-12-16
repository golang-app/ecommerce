package app

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type ProductService struct {
	storage ProductStorage
}

type ProductStorage interface {
	All(ctx context.Context) ([]domain.Product, error)
	Add(ctx context.Context, p domain.Product) error
	Find(ctx context.Context, id string) (domain.Product, error)
	Reserve(ctx context.Context, name string) error
}

func NewProductService(s ProductStorage) ProductService {
	return ProductService{storage: s}
}

func (ps ProductService) AllProducts(ctx context.Context) ([]domain.Product, error) {
	return ps.storage.All(ctx)
}

func (ps ProductService) Find(ctx context.Context, id string) (domain.Product, error) {
	return ps.storage.Find(ctx, id)
}

func (ps ProductService) ProductBuilder() productBuilder {
	return productBuilder{
		reservation: productIdReservation{
			storage: ps.storage,
		},
	}
}
