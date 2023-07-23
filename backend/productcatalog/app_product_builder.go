package productcatalog

import (
	"context"
	"fmt"
)

type productBuilder struct {
	id          string
	name        string
	description string
	price       Price
	thumbnail   string
}

func NewProductBuilder() productBuilder {
	return productBuilder{}
}

func (pb productBuilder) Build(ctx context.Context) (Product, error) {
	id, err := NewProductId(pb.id)
	if err != nil {
		return Product{}, fmt.Errorf("cannot create product: %w", err)
	}

	return NewProduct(id, pb.name, pb.description, pb.price, pb.thumbnail)
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

func (pb productBuilder) WithPrice(p Price) productBuilder {
	pb.price = p
	return pb
}

func (pb productBuilder) WithThumbnail(thumbnail string) productBuilder {
	pb.thumbnail = thumbnail
	return pb
}
