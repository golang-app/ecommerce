package cart_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

var storage app.CartStorage

func TestAddMinusItemsToCart(t *testing.T) {
	// given
	pID := "productID"
	serv := newCartServiceBuilder().WithProduct(pID, 10).build()
	ctx := context.Background()
	sessID := sessionID()

	// when
	err := serv.AddToCart(ctx, sessID, pID, -1)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// when
	cart, err := serv.Get(ctx, sessID)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// then
	if len(cart.Items()) != 0 {
		t.Errorf("expected 0 items, got %d", len(cart.Items()))
	}

	// wee add 1 element to the cart and then remove it by adding `-1`
	err = serv.AddToCart(ctx, sessID, pID, 1)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}
	err = serv.AddToCart(ctx, sessID, pID, -2)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// then
	if len(cart.Items()) != 0 {
		t.Errorf("expected 0 items, got %d", len(cart.Items()))
	}
}

func TestAddItemToCartSuccessfully(t *testing.T) {
	// given
	pID := "productID"
	serv := newCartServiceBuilder().WithProduct(pID, 10).build()
	ctx := context.Background()
	sessID := sessionID()

	// when
	err := serv.AddToCart(ctx, sessID, pID, 10)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// when
	cart, err := serv.Get(ctx, sessID)
	if err != nil {
		t.Errorf("could not add product to the cart: %s", err)
	}

	// then
	if len(cart.Items()) != 1 {
		t.Errorf("expected 10 items, got %d", len(cart.Items()))
	}

	items := cart.Items()

	if items[0].Product().ID() != pID {
		t.Errorf("expected product ID to be '%s', got '%s'", pID, items[0].Product().ID())
	}

	if items[0].Quantity() != 10 {
		t.Errorf("expected quantity to be 10, got %d", items[0].Quantity())
	}
}

func TestCannotAddNotExistingProductToTheCart(t *testing.T) {
	// given
	pID := "productID"
	serv := newCartServiceBuilder().build()
	ctx := context.Background()
	sessID := "sessionID"

	// when
	err := serv.AddToCart(ctx, sessID, pID, 10)

	// then
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

type cartServiceBuilder struct {
	products map[string]domain.Product
}

func newCartServiceBuilder() cartServiceBuilder {
	return cartServiceBuilder{
		products: map[string]domain.Product{},
	}
}

func (b cartServiceBuilder) build() app.CartService {
	return app.NewCartService(storage, mockProductCatalog(b))
}

func (b cartServiceBuilder) WithProduct(pID string, price float64) cartServiceBuilder {
	b.products[pID] = domain.NewProduct(pID, "test name", price, "PLN")
	return b
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

func sessionID() (uuid string) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	uuid = fmt.Sprintf("user_%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	return
}
