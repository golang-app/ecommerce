package cart_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

func TestAddItemToCart(t *testing.T) {
	serv := app.NewCartService(nil, nil)
	serv.Add(context.Background(), "cartID", "productID", 1)
}

type mockProductCatalog struct {
}

func (m mockProductCatalog) Find(ctx context.Context, productID string) (domain.Product, error) {
	return domain.Product{}, nil
}
