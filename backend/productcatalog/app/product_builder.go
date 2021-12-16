package app

import (
	"context"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type productBuilder struct {
	reservation productIdReservation

	name        string
	description string
	price       domain.Price
	thumbnail   string
}

func NewProductBuilder(storage productIDReservationStorage) productBuilder {
	return productBuilder{reservation: productIdReservation{
		storage: storage,
	}}
}

func (pb productBuilder) Build(ctx context.Context) (domain.Product, error) {
	id, err := pb.reservation.Reserve(ctx, pb.name)
	if err != nil {
		return domain.Product{}, fmt.Errorf("cannot build the product: %w", err)
	}

	return domain.NewProduct(id, pb.name, pb.description, pb.price, pb.thumbnail)
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
