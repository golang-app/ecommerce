package productcatalog_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/matryer/is"
)

type productStorage interface {
	All(ctx context.Context) ([]productcatalog.Product, error)
	Add(ctx context.Context, p productcatalog.Product) error
	Find(ctx context.Context, id string) (productcatalog.Product, error)
}

var storage productStorage

func TestFetchingProductsInTheCatalog(t *testing.T) {
	is := is.New(t)
	// given
	ctx := context.Background()
	appServ := productcatalog.NewProductService(storage)

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
	appServ := productcatalog.NewProductService(storage)

	// when
	_, err := appServ.Find(ctx, "i-dont-exist")

	// then
	is.True(errors.Is(err, productcatalog.ErrProductNotFound))
}

func productEquals(p1, p2 productcatalog.Product) error {
	if p1.ID() != p2.ID() {
		return errors.New("id misatch")
	}
	if p1.Description() != p2.Description() {
		return errors.New("description misatch")
	}
	if p1.Thumbnail() != p2.Thumbnail() {
		return errors.New("thumbnail misatch")
	}
	if p1.Price() != p2.Price() {
		return errors.New("price misatch")
	}

	return nil
}

func buildProduct(ctx context.Context, storage productcatalog.ProductStorage) (productcatalog.Product, error) {
	pb := productcatalog.NewProductBuilder()
	price := productcatalog.NewPrice(234, "USD")
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
