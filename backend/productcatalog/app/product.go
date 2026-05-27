package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

type ProductService struct {
	storage ProductStorage
}

type ProductStorage interface {
	All(ctx context.Context) ([]domain.Product, error)
	Add(ctx context.Context, p domain.Product) error
	Find(ctx context.Context, id string) (domain.Product, error)
	FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error)
	AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error
	AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
	ListProducts(ctx context.Context, q ProductQuery) ([]domain.Product, error)
	Categories(ctx context.Context) ([]domain.Category, error)
	Facets(ctx context.Context, categorySlug string) ([]Facet, error)

	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, id string) error

	AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error)
	CreateAttributeType(ctx context.Context, t domain.AttributeType) error
	UpdateAttributeType(ctx context.Context, t domain.AttributeType) error
	DeleteAttributeType(ctx context.Context, id string) error
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

// FindVariant resolves a variant id to its variant and owning product.
func (ps ProductService) FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error) {
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

// List returns the products matching the given listing-page query.
func (ps ProductService) List(ctx context.Context, q ProductQuery) ([]domain.Product, error) {
	return ps.storage.ListProducts(ctx, q)
}

// Categories returns all catalog categories in display order.
func (ps ProductService) Categories(ctx context.Context) ([]domain.Category, error) {
	return ps.storage.Categories(ctx)
}

// Facets returns the available filter facets, optionally scoped to a category.
func (ps ProductService) Facets(ctx context.Context, categorySlug string) ([]Facet, error) {
	return ps.storage.Facets(ctx, categorySlug)
}

// CreateCategory validates and persists a new category. The id equals the slug
// and the position is appended after the current categories.
func (ps ProductService) CreateCategory(ctx context.Context, name, slug string) error {
	existing, err := ps.storage.Categories(ctx)
	if err != nil {
		return err
	}
	c, err := domain.NewCategory(slug, name, slug, len(existing)+1)
	if err != nil {
		return err
	}
	return ps.storage.CreateCategory(ctx, c)
}

// UpdateCategory validates and persists changes to an existing category.
func (ps ProductService) UpdateCategory(ctx context.Context, id, name, slug string, position int) error {
	c, err := domain.NewCategory(id, name, slug, position)
	if err != nil {
		return err
	}
	return ps.storage.UpdateCategory(ctx, c)
}

// DeleteCategory removes a category (its product links cascade in storage).
func (ps ProductService) DeleteCategory(ctx context.Context, id string) error {
	return ps.storage.DeleteCategory(ctx, id)
}

// AttributeTypes returns every attribute type in display order (admin view).
func (ps ProductService) AttributeTypes(ctx context.Context) ([]domain.AttributeType, error) {
	return ps.storage.AllAttributeTypes(ctx)
}

// CreateAttributeType validates and persists a new attribute type. The id is a
// slug derived from the name and the position is appended after existing types.
func (ps ProductService) CreateAttributeType(ctx context.Context, name, unit string, kind domain.AttributeKind, filterable bool) error {
	existing, err := ps.storage.AllAttributeTypes(ctx)
	if err != nil {
		return err
	}
	id := slugify(name)
	t, err := domain.NewAttributeType(id, name, unit, kind, filterable, len(existing)+1)
	if err != nil {
		return err
	}
	return ps.storage.CreateAttributeType(ctx, t)
}

// UpdateAttributeType validates and persists changes to an attribute type.
func (ps ProductService) UpdateAttributeType(ctx context.Context, id, name, unit string, kind domain.AttributeKind, filterable bool, position int) error {
	t, err := domain.NewAttributeType(id, name, unit, kind, filterable, position)
	if err != nil {
		return err
	}
	return ps.storage.UpdateAttributeType(ctx, t)
}

// DeleteAttributeType removes an attribute type (its product links cascade).
func (ps ProductService) DeleteAttributeType(ctx context.Context, id string) error {
	return ps.storage.DeleteAttributeType(ctx, id)
}

// slugify turns a display name into a url-safe id: lowercase, spaces to
// hyphens, dropping any character that is not a lowercase letter, digit or
// hyphen, and collapsing repeated/edge hyphens.
func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// defaultStock is given to a simple product's auto-created default variant.
const defaultStock = 100

func (ps ProductService) Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	pId, err := domain.NewProductId(id)
	if err != nil {
		return err
	}

	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	priceVO, err := domain.NewPrice(priceMinorUnits, cur)
	if err != nil {
		return fmt.Errorf("invalid price: %w", err)
	}

	p, err := domain.NewProduct(pId, name, desc, priceVO, thumbnail)
	if err != nil {
		return err
	}

	if err = ps.storage.Add(ctx, p); err != nil {
		return err
	}

	// A simple product is purchasable through a single default variant
	// carrying its price (no options) and the product image.
	defaultVariant := domain.NewVariant("var-"+id, id, thumbnail, nil, priceVO, defaultStock)
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
	pID, err := domain.NewProductId(id)
	if err != nil {
		return err
	}
	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return fmt.Errorf("invalid currency: %w", err)
	}

	base := variants[0].Price
	for _, v := range variants[1:] {
		if v.Price < base {
			base = v.Price
		}
	}
	basePrice, err := domain.NewPrice(base, cur)
	if err != nil {
		return fmt.Errorf("invalid base price: %w", err)
	}

	product, err := domain.NewProduct(pID, name, desc, basePrice, thumbnail)
	if err != nil {
		return err
	}
	if err = ps.storage.Add(ctx, product); err != nil {
		return err
	}

	for i, ot := range optionTypes {
		if err = ps.storage.AddOptionType(ctx, id, i, domain.NewOptionType(ot.Name, ot.Values)); err != nil {
			return err
		}
	}
	for i, v := range variants {
		price, err := domain.NewPrice(v.Price, cur)
		if err != nil {
			return fmt.Errorf("invalid variant price: %w", err)
		}
		if err = ps.storage.AddVariant(ctx, id, i, domain.NewVariant(v.ID, v.SKU, v.Image, v.Options, price, v.Stock)); err != nil {
			return err
		}
	}
	return nil
}
