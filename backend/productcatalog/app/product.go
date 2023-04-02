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

func (ps ProductService) Add(ctx context.Context, id, name, desc string, price float64, currency string) error {
	pId, err := domain.NewProductId(id)
	if err != nil {
		return err
	}

	p, err := domain.NewProduct(pId, name, desc, domain.NewPrice(price, currency), "")
	if err != nil {
		return err
	}

	err = ps.storage.Add(ctx, p)
	if err != nil {
		return err
	}

	return nil
}
