package integration_test

import (
	"context"
	"io"
	"testing"

	cartadapter "github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	cartapp "github.com/bkielbasa/go-ecommerce/backend/cart/app"
	cartdomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	productcatalogintegration "github.com/bkielbasa/go-ecommerce/backend/productcatalog/integration"
	"github.com/sirupsen/logrus"
)

// catalogAdapter bridges productcatalog storage into cart's productCatalog port.
type catalogAdapter struct {
	storage pcapp.ProductStorage
}

func (a catalogAdapter) Find(ctx context.Context, variantID string) (cartdomain.Product, error) {
	owner, v, err := a.storage.FindVariant(ctx, variantID)
	if err != nil {
		return cartdomain.Product{}, err
	}
	label := v.Label(owner.OptionTypes())
	name := owner.Name()
	if label != "" {
		name = name + " — " + label
	}
	cur, err := cartdomain.NewCurrency(string(v.Price().Currency()))
	if err != nil {
		return cartdomain.Product{}, err
	}
	return cartdomain.NewProduct(v.ID(), name, v.Price().Amount(), cur), nil
}

func TestCartSync_OnCatalogNameAndPriceChanges(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	bus := eventbus.New(logger)

	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage).WithPublisher(bus)

	cartStorage := cartadapter.NewInMemory()
	cartSrv := cartapp.NewCartService(cartStorage, catalogAdapter{storage: pcStorage})

	// Wire subscribers as in main.go
	bus.Subscribe(
		productcatalogintegration.ProductNameChanged{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			evt := e.(productcatalogintegration.ProductNameChanged)
			return cartSrv.UpdateItemName(ctx, evt.VariantID, evt.NewName)
		},
	)

	bus.Subscribe(
		productcatalogintegration.ProductPriceChanged{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			evt := e.(productcatalogintegration.ProductPriceChanged)
			return cartSrv.UpdateItemPrice(ctx, evt.VariantID, evt.PriceAmount, evt.PriceCurrency)
		},
	)

	// 1. Add simple product to catalog
	pID := "prod-coffee-mug"
	variantID := "var-" + pID
	err := catalogSrv.Add(ctx, pID, "Ceramic Coffee Mug", "A lovely mug", 1200, "USD", "mug.jpg")
	if err != nil {
		t.Fatalf("catalog.Add: %v", err)
	}

	// 2. Add product to user's cart
	sessionID := "user-session-123"
	err = cartSrv.AddToCart(ctx, sessionID, variantID, 2)
	if err != nil {
		t.Fatalf("cart.AddToCart: %v", err)
	}

	// 3. Verify initial cart state
	cart, err := cartSrv.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("cart.Get: %v", err)
	}
	if len(cart.Items()) != 1 {
		t.Fatalf("expected 1 item, got %d", len(cart.Items()))
	}
	if cart.Items()[0].Product().Name() != "Ceramic Coffee Mug" {
		t.Fatalf("expected name 'Ceramic Coffee Mug', got %q", cart.Items()[0].Product().Name())
	}
	if cart.Items()[0].Product().Price().Amount() != 1200 {
		t.Fatalf("expected price 1200, got %d", cart.Items()[0].Product().Price().Amount())
	}
	if cart.TotalPrice().Amount() != 2400 {
		t.Fatalf("expected total 2400, got %d", cart.TotalPrice().Amount())
	}

	// 4. Update product name and price in the catalog
	err = catalogSrv.UpdateProduct(ctx, pID, "Premium Ceramic Coffee Mug", "A lovely mug", 1500, "USD", "mug.jpg")
	if err != nil {
		t.Fatalf("catalog.UpdateProduct: %v", err)
	}

	// 5. Inspect cart: item name, item unit price, and total must reflect the catalog change
	cart, err = cartSrv.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("cart.Get after update: %v", err)
	}
	if cart.Items()[0].Product().Name() != "Premium Ceramic Coffee Mug" {
		t.Fatalf("expected updated name 'Premium Ceramic Coffee Mug', got %q", cart.Items()[0].Product().Name())
	}
	if cart.Items()[0].Product().Price().Amount() != 1500 {
		t.Fatalf("expected updated price 1500, got %d", cart.Items()[0].Product().Price().Amount())
	}
	if cart.TotalPrice().Amount() != 3000 {
		t.Fatalf("expected updated total 3000, got %d", cart.TotalPrice().Amount())
	}
}

func TestCartSync_OnVariantPriceAndNameChanges(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	bus := eventbus.New(logger)

	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage).WithPublisher(bus)

	cartStorage := cartadapter.NewInMemory()
	cartSrv := cartapp.NewCartService(cartStorage, catalogAdapter{storage: pcStorage})

	bus.Subscribe(
		productcatalogintegration.ProductNameChanged{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			evt := e.(productcatalogintegration.ProductNameChanged)
			return cartSrv.UpdateItemName(ctx, evt.VariantID, evt.NewName)
		},
	)

	bus.Subscribe(
		productcatalogintegration.ProductPriceChanged{}.EventName(),
		func(ctx context.Context, e eventbus.Event) error {
			evt := e.(productcatalogintegration.ProductPriceChanged)
			return cartSrv.UpdateItemPrice(ctx, evt.VariantID, evt.PriceAmount, evt.PriceCurrency)
		},
	)

	// Add variant product
	pID := "prod-hoodie"
	varS := "var-hoodie-s"
	varM := "var-hoodie-m"
	err := catalogSrv.AddVariantProduct(ctx, pID, "Comfy Hoodie", "Hoodie description", "USD", "hoodie.jpg",
		[]pcapp.OptionTypeInput{
			{Name: "Size", Values: []string{"S", "M"}},
		},
		[]pcapp.VariantInput{
			{ID: varS, SKU: "HOOD-S", Price: 4000, Stock: 10, Options: map[string]string{"Size": "S"}},
			{ID: varM, SKU: "HOOD-M", Price: 4500, Stock: 10, Options: map[string]string{"Size": "M"}},
		},
	)
	if err != nil {
		t.Fatalf("catalog.AddVariantProduct: %v", err)
	}

	// Two different sessions add different sizes
	sess1 := "user-1"
	sess2 := "user-2"
	if err := cartSrv.AddToCart(ctx, sess1, varS, 1); err != nil {
		t.Fatalf("cart.AddToCart user 1: %v", err)
	}
	if err := cartSrv.AddToCart(ctx, sess2, varM, 2); err != nil {
		t.Fatalf("cart.AddToCart user 2: %v", err)
	}

	// Update only variant S's price
	if err := catalogSrv.UpdateVariant(ctx, varS, "HOOD-S", "hoodie.jpg", 4200, "USD", 10); err != nil {
		t.Fatalf("catalog.UpdateVariant: %v", err)
	}

	// Verify user 1's cart updated to 4200, while user 2's cart remains 4500 * 2 = 9000
	cart1, err := cartSrv.Get(ctx, sess1)
	if err != nil {
		t.Fatalf("cart1.Get: %v", err)
	}
	if cart1.Items()[0].Product().Price().Amount() != 4200 {
		t.Fatalf("expected user 1 price 4200, got %d", cart1.Items()[0].Product().Price().Amount())
	}

	cart2, err := cartSrv.Get(ctx, sess2)
	if err != nil {
		t.Fatalf("cart2.Get: %v", err)
	}
	if cart2.Items()[0].Product().Price().Amount() != 4500 {
		t.Fatalf("expected user 2 price unchanged at 4500, got %d", cart2.Items()[0].Product().Price().Amount())
	}
	if cart2.TotalPrice().Amount() != 9000 {
		t.Fatalf("expected user 2 total 9000, got %d", cart2.TotalPrice().Amount())
	}

	// Now update the parent product's name
	if err := catalogSrv.UpdateProduct(ctx, pID, "Ultra Comfy Hoodie", "Updated desc", 4000, "USD", "hoodie.jpg"); err != nil {
		t.Fatalf("catalog.UpdateProduct: %v", err)
	}

	// Verify both carts reflect the new product name with their respective variant labels
	cart1, _ = cartSrv.Get(ctx, sess1)
	if cart1.Items()[0].Product().Name() != "Ultra Comfy Hoodie — S" {
		t.Fatalf("expected cart1 name 'Ultra Comfy Hoodie — S', got %q", cart1.Items()[0].Product().Name())
	}

	cart2, _ = cartSrv.Get(ctx, sess2)
	if cart2.Items()[0].Product().Name() != "Ultra Comfy Hoodie — M" {
		t.Fatalf("expected cart2 name 'Ultra Comfy Hoodie — M', got %q", cart2.Items()[0].Product().Name())
	}
}
