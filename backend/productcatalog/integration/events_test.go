package integration_test

import (
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/integration"
)

func TestProductNameChanged_EventName(t *testing.T) {
	evt := integration.ProductNameChanged{
		ProductID: "prod-1",
		VariantID: "var-1",
		NewName:   "Updated Name",
		At:        time.Now(),
	}
	if evt.EventName() != "productcatalog.ProductNameChanged" {
		t.Fatalf("expected productcatalog.ProductNameChanged, got %s", evt.EventName())
	}
}

func TestProductPriceChanged_EventName(t *testing.T) {
	evt := integration.ProductPriceChanged{
		ProductID:     "prod-1",
		VariantID:     "var-1",
		PriceAmount:   1999,
		PriceCurrency: "USD",
		At:            time.Now(),
	}
	if evt.EventName() != "productcatalog.ProductPriceChanged" {
		t.Fatalf("expected productcatalog.ProductPriceChanged, got %s", evt.EventName())
	}
}
