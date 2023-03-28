package app

import (
	"context"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type productBuilder struct {
	id          string
	name        string
	description string
	price       domain.Price
	thumbnail   string
}

func NewProductBuilder(storage productIDReservationStorage) productBuilder {
	return productBuilder{}
}

func (pb productBuilder) Build(ctx context.Context) (domain.Product, error) {
	id, err := domain.NewProductId(pb.id)
	if err != nil {
		return domain.Product{}, fmt.Errorf("cannot create product: %w", err)
	}

	return domain.NewProduct(id, pb.name, pb.description, pb.price, pb.thumbnail)
}

func (pb productBuilder) WithID(id string) productBuilder {
	pb.id = id
	return pb
}

func (pb productBuilder) WithName(name string) productBuilder {
	pb.name = name
	return pb
}

func (pb productBuilder) WithDescription(description string) productBuilder {
	pb.description = description
	return pb
}

func (pb productBuilder) WithPrice(p domain.Price) productBuilder {
	pb.price = p
	return pb
}

func (pb productBuilder) WithThumbnail(thumbnail string) productBuilder {
	pb.thumbnail = thumbnail
	return pb
}
