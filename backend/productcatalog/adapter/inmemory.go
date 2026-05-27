package adapter

import (
	"context"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type inMemory struct {
	products    []domain.Product
	optionTypes map[string][]domain.OptionType
	variants    map[string][]domain.Variant
}

func NewInMemory() *inMemory {
	return &inMemory{
		optionTypes: map[string][]domain.OptionType{},
		variants:    map[string][]domain.Variant{},
	}
}

func (im *inMemory) Add(ctx context.Context, p domain.Product) error {
	im.products = append(im.products, p)
	return nil
}

func (im *inMemory) AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error {
	im.optionTypes[productID] = append(im.optionTypes[productID], ot)
	return nil
}

func (im *inMemory) AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error {
	im.variants[productID] = append(im.variants[productID], v)
	return nil
}

func (im *inMemory) hydrate(p domain.Product) domain.Product {
	return p.WithCatalog(im.optionTypes[string(p.ID())], im.variants[string(p.ID())])
}

func (im *inMemory) All(ctx context.Context) ([]domain.Product, error) {
	out := make([]domain.Product, 0, len(im.products))
	for _, p := range im.products {
		out = append(out, im.hydrate(p))
	}
	return out, nil
}

func (im *inMemory) Find(ctx context.Context, id string) (domain.Product, error) {
	for _, p := range im.products {
		if string(p.ID()) == id {
			return im.hydrate(p), nil
		}
	}
	return domain.Product{}, domain.ErrProductNotFound
}

func (im *inMemory) find(variantID string) (string, int, bool) {
	for pid, vs := range im.variants {
		for i, v := range vs {
			if v.ID() == variantID {
				return pid, i, true
			}
		}
	}
	return "", 0, false
}

// Reserve checks availability for all variants first, then decrements — so an
// insufficient item leaves everything untouched.
func (im *inMemory) Reserve(ctx context.Context, quantities map[string]int) error {
	for id, qty := range quantities {
		pid, i, ok := im.find(id)
		if !ok || im.variants[pid][i].Stock() < qty {
			return domain.ErrInsufficientStock
		}
	}
	for id, qty := range quantities {
		pid, i, _ := im.find(id)
		v := im.variants[pid][i]
		im.variants[pid][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), v.Options(), v.Price(), v.Stock()-qty)
	}
	return nil
}

func (im *inMemory) Release(ctx context.Context, quantities map[string]int) error {
	for id, qty := range quantities {
		if pid, i, ok := im.find(id); ok {
			v := im.variants[pid][i]
			im.variants[pid][i] = domain.NewVariant(v.ID(), v.SKU(), v.Image(), v.Options(), v.Price(), v.Stock()+qty)
		}
	}
	return nil
}

func (im *inMemory) FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error) {
	for _, p := range im.products {
		full := im.hydrate(p)
		if v, ok := full.Variant(variantID); ok {
			return full, v, nil
		}
	}
	return domain.Product{}, domain.Variant{}, domain.ErrProductNotFound
}
