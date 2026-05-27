package productcatalog

import (
	"context"
)

type inMemory struct {
	products    []Product
	optionTypes map[string][]OptionType
	variants    map[string][]Variant
}

func NewInMemory() *inMemory {
	return &inMemory{
		optionTypes: map[string][]OptionType{},
		variants:    map[string][]Variant{},
	}
}

func (im *inMemory) Add(ctx context.Context, p Product) error {
	im.products = append(im.products, p)
	return nil
}

func (im *inMemory) AddOptionType(ctx context.Context, productID string, position int, ot OptionType) error {
	im.optionTypes[productID] = append(im.optionTypes[productID], ot)
	return nil
}

func (im *inMemory) AddVariant(ctx context.Context, productID string, position int, v Variant) error {
	im.variants[productID] = append(im.variants[productID], v)
	return nil
}

func (im *inMemory) hydrate(p Product) Product {
	return p.WithCatalog(im.optionTypes[string(p.ID())], im.variants[string(p.ID())])
}

func (im *inMemory) All(ctx context.Context) ([]Product, error) {
	out := make([]Product, 0, len(im.products))
	for _, p := range im.products {
		out = append(out, im.hydrate(p))
	}
	return out, nil
}

func (im *inMemory) Find(ctx context.Context, id string) (Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return im.hydrate(p), nil
		}
	}
	return Product{}, ErrProductNotFound
}

func (im *inMemory) FindVariant(ctx context.Context, variantID string) (Product, Variant, error) {
	for _, p := range im.products {
		full := im.hydrate(p)
		if v, ok := full.Variant(variantID); ok {
			return full, v, nil
		}
	}
	return Product{}, Variant{}, ErrProductNotFound
}
