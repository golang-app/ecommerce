package productcatalog_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/matryer/is"
)

var storage app.ProductStorage

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
	if p1.Price() != p2.Price() {
		return errors.New("price misatch")
	}

	return nil
}

func buildProduct(ctx context.Context, storage app.ProductStorage) (domain.Product, error) {
	pb := app.NewProductBuilder(storage)
	price := domain.NewPrice(234, "USD")
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
