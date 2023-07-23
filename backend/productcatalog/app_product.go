package productcatalog

import (
	"context"
)

type ProductService struct {
	storage ProductStorage
}

type ProductStorage interface {
	All(ctx context.Context) ([]Product, error)
	Add(ctx context.Context, p Product) error
	Find(ctx context.Context, id string) (Product, error)
}

func NewProductService(s ProductStorage) ProductService {
	return ProductService{storage: s}
}

func (ps ProductService) AllProducts(ctx context.Context) ([]Product, error) {
	return ps.storage.All(ctx)
}

func (ps ProductService) Find(ctx context.Context, id string) (Product, error) {
	return ps.storage.Find(ctx, id)
}

func (ps ProductService) Add(ctx context.Context, id, name, desc string, price float64, currency string) error {
	pId, err := NewProductId(id)
	if err != nil {
		return err
	}

	p, err := NewProduct(pId, name, desc, NewPrice(price, currency), "")
	if err != nil {
		return err
	}

	err = ps.storage.Add(ctx, p)
	if err != nil {
		return err
	}

	return nil
}
