package productcatalog

import (
	"context"
	"fmt"
)

type ProductService struct {
	storage ProductStorage
}

type ProductStorage interface {
	All(ctx context.Context) ([]Product, error)
	Add(ctx context.Context, p Product) error
	Find(ctx context.Context, id string) (Product, error)
	FindVariant(ctx context.Context, variantID string) (Product, Variant, error)
	AddOptionType(ctx context.Context, productID string, position int, ot OptionType) error
	AddVariant(ctx context.Context, productID string, position int, v Variant) error
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
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

// FindVariant resolves a variant id to its variant and owning product.
func (ps ProductService) FindVariant(ctx context.Context, variantID string) (Product, Variant, error) {
	return ps.storage.FindVariant(ctx, variantID)
}

// Reserve atomically decrements stock for the given variants (variant id ->
// quantity). All-or-nothing: returns ErrInsufficientStock if any variant is
// short, leaving stock untouched.
func (ps ProductService) Reserve(ctx context.Context, quantities map[string]int) error {
	return ps.storage.Reserve(ctx, quantities)
}

// Release returns previously-reserved stock (e.g. after a failed payment).
func (ps ProductService) Release(ctx context.Context, quantities map[string]int) error {
	return ps.storage.Release(ctx, quantities)
}

// defaultStock is given to a simple product's auto-created default variant.
const defaultStock = 100

func (ps ProductService) Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	pId, err := NewProductId(id)
	if err != nil {
		return err
	}

	cur, err := NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	priceVO, err := NewPrice(priceMinorUnits, cur)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	p, err := NewProduct(pId, name, desc, priceVO, thumbnail)
	if err != nil {
		return err
	}

	if err = ps.storage.Add(ctx, p); err != nil {
		return err
	}

	// A simple product is purchasable through a single default variant
	// carrying its price (no options) and the product image.
	defaultVariant := NewVariant("var-"+id, id, thumbnail, nil, priceVO, defaultStock)
	return ps.storage.AddVariant(ctx, id, 0, defaultVariant)
}

// OptionTypeInput / VariantInput describe a product's options and variants for
// AddVariantProduct (used by seeding).
type OptionTypeInput struct {
	Name   string
	Values []string
}

type VariantInput struct {
	ID      string
	SKU     string
	Image   string
	Options map[string]string
	Price   int64
	Stock   int
}

// AddVariantProduct creates a product with explicit option types and variants
// (each independently priced). The product's base price is the lowest variant
// price, used for "from $X" display.
func (ps ProductService) AddVariantProduct(ctx context.Context, id, name, desc, currency, thumbnail string, optionTypes []OptionTypeInput, variants []VariantInput) error {
	if len(variants) == 0 {
		return fmt.Errorf("a variant product needs at least one variant")
	}
	pID, err := NewProductId(id)
	if err != nil {
		return err
	}
	cur, err := NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	base := variants[0].Price
	for _, v := range variants[1:] {
		if v.Price < base {
			base = v.Price
		}
	}
	basePrice, err := NewPrice(base, cur)
	if err != nil {
		return fmt.Errorf("invalid base price: %w", err)
	}

	product, err := NewProduct(pID, name, desc, basePrice, thumbnail)
	if err != nil {
		return err
	}
	if err = ps.storage.Add(ctx, product); err != nil {
		return err
	}

	for i, ot := range optionTypes {
		if err = ps.storage.AddOptionType(ctx, id, i, NewOptionType(ot.Name, ot.Values)); err != nil {
			return err
		}
	}
	for i, v := range variants {
		price, err := NewPrice(v.Price, cur)
		if err != nil {
			return fmt.Errorf("invalid variant price: %w", err)
		}
		if err = ps.storage.AddVariant(ctx, id, i, NewVariant(v.ID, v.SKU, v.Image, v.Options, price, v.Stock)); err != nil {
			return err
		}
	}
	return nil
}
