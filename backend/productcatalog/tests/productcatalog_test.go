package tests

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/matryer/is"
)

type productStorage interface {
	app.Storage
	Reserve(ctx context.Context, name string) error
}

var storage productStorage

func TestFetchingProductsInTheCatalog(t *testing.T) {
	is := is.New(t)
	// given
	ctx := context.Background()
	appServ := app.NewProduct(storage)
	pb := domain.NewProductBuilder(domain.NewProductIdReservation(storage))
	price := domain.NewPrice(234, "USD")
	p, err := pb.Build(ctx, "Test product", "description of the test product", price, "http://some.url")
	is.NoErr(err)
	err = storage.Add(ctx, p)
	is.NoErr(err)

	// when
	fetched, err := appServ.Find(ctx, string(p.ID()))

	// then
	is.NoErr(err)
	is.Equal(p.ID(), fetched.ID())
	is.Equal(p.Name(), fetched.Name())
	is.Equal(p.Description(), fetched.Description())
	is.Equal(p.Thumbnail(), fetched.Thumbnail())
	is.Equal(p.Price(), fetched.Price())
}
