package cart_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

func TestAddItemToCart(t *testing.T) {
	// given
	pID := "productID"
	serv := buildCartService(pID)

	// when
	err := serv.Add(context.Background(), pID, 10)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// then
	tp, err := serv.TotalPrice()
	if err != nil {
		t.Errorf("could not get total price: %s", err)
	}

	expected := 100.0
	if tp != expected {
		t.Errorf("expected total price to be %f, got %f", expected, tp)
	}
}

func buildCartService(pid string) app.CartService {
	c := domain.NewCart(newMockPrice())
	return app.NewCartService(c, mockProductCatalog{
		products: map[string]domain.Product{
			pid: domain.NewProduct(pid, 10),
		},
	})
}

func newMockPrice() mockPriceService {
	return mockPriceService{}
}

type mockPriceService struct {
}

func (m mockPriceService) PriceFor(productID string) (float64, error) {
	return 1, nil
}

type mockProductCatalog struct {
	products map[string]domain.Product
}

func (m mockProductCatalog) Find(ctx context.Context, productID string) (domain.Product, error) {
	p, ok := m.products[productID]
	if !ok {
		return domain.Product{}, fmt.Errorf("could not find product %s in the product catalog", productID)
	}

	return p, nil
}
