package app_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/matryer/is"
)

type productStorage interface {
	All(ctx context.Context) ([]domain.Product, error)
	Newest(ctx context.Context, limit int) ([]domain.Product, error)
	Add(ctx context.Context, p domain.Product) error
	UpdateProduct(ctx context.Context, p domain.Product) error
	DeleteProduct(ctx context.Context, id string) error
	SetVariantStock(ctx context.Context, variantID string, stock int) error
	SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error
	SetProductAttributes(ctx context.Context, productID string, values []app.AttributeAssignment) error
	SetProductAttributeSet(ctx context.Context, productID, setID string) error
	Find(ctx context.Context, id string) (domain.Product, error)
	FindVariant(ctx context.Context, variantID string) (domain.Product, domain.Variant, error)
	AddOptionType(ctx context.Context, productID string, position int, ot domain.OptionType) error
	AddProductOptionType(ctx context.Context, productID, optionTypeID, name string, position int, values []string, variantDefault string) error
	UpdateProductOptionType(ctx context.Context, productID, currentName, newName string, values []string) error
	DeleteProductOptionType(ctx context.Context, productID, name string) error
	AddVariant(ctx context.Context, productID string, position int, v domain.Variant) error
	UpdateVariant(ctx context.Context, variantID, sku, image string, priceAmount int64, currency string, stock int) error
	DeleteVariant(ctx context.Context, variantID string) error
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
	ListProducts(ctx context.Context, q app.ProductQuery) ([]domain.Product, error)
	Categories(ctx context.Context) ([]domain.Category, error)
	Facets(ctx context.Context, categorySlug string) ([]app.Facet, error)
	CreateCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, id string) error
	AllAttributeTypes(ctx context.Context) ([]domain.AttributeType, error)
	CreateAttributeType(ctx context.Context, t domain.AttributeType) error
	UpdateAttributeType(ctx context.Context, t domain.AttributeType) error
	DeleteAttributeType(ctx context.Context, id string) error
	AllAttributeSets(ctx context.Context) ([]domain.AttributeSet, error)
	FindAttributeSet(ctx context.Context, id string) (domain.AttributeSet, error)
	CreateAttributeSet(ctx context.Context, s domain.AttributeSet) error
	UpdateAttributeSet(ctx context.Context, s domain.AttributeSet) error
	DeleteAttributeSet(ctx context.Context, id string) error
	SetAttributeSetItems(ctx context.Context, setID string, attributeTypeIDs []string) error
	InsertStockMovement(ctx context.Context, variantID string, delta int, reason, refOrderID string) error
	ListStockMovements(ctx context.Context, variantID string, limit int) ([]domain.StockMovement, error)
}

var storage productStorage

func TestFetchingProductsInTheCatalog(t *testing.T) {
	is := is.New(t)
	// given
	ctx := context.Background()
	appServ := app.NewProductService(storage)

	p, err := buildProduct(ctx, storage)
	is.NoErr(err)
	err = storage.Add(ctx, p)
	is.NoErr(err)

	// when
	fetched, err := appServ.Find(ctx, string(p.ID()))

	// then
	is.NoErr(err)
	is.NoErr(productEquals(p, fetched))
}

func TestFetchingNonExistingProduct(t *testing.T) {
	is := is.New(t)
	// given
	ctx := context.Background()
	appServ := app.NewProductService(storage)

	// when
	_, err := appServ.Find(ctx, "i-dont-exist")

	// then
	is.True(errors.Is(err, domain.ErrProductNotFound))
}

func productEquals(p1, p2 domain.Product) error {
	if p1.ID() != p2.ID() {
		return errors.New("id misatch")
	}
	if p1.Description() != p2.Description() {
		return errors.New("description misatch")
	}
	if p1.Thumbnail() != p2.Thumbnail() {
		return errors.New("thumbnail misatch")
	}
	if !p1.Price().Equals(p2.Price()) {
		return errors.New("price misatch")
	}

	return nil
}

func buildProduct(ctx context.Context, storage app.ProductStorage) (domain.Product, error) {
	pb := app.NewProductBuilder()
	price := domain.MustNewPrice(234, domain.MustNewCurrency("USD"))
	pb = pb.WithName("Test product").
		WithID(randomID()).
		WithDescription("description of the test product").
		WithPrice(price).
		WithThumbnail("http://some.url")

	return pb.Build(ctx)
}

func randomID() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
