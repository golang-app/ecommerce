package app_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/integration"
	"github.com/matryer/is"
)

type eventSpy struct {
	events []eventbus.Event
}

func (s *eventSpy) Publish(_ context.Context, e eventbus.Event) {
	s.events = append(s.events, e)
}

func TestProductService_PublishesEventsOnUpdateProduct(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	spy := &eventSpy{}
	service := app.NewProductService(storage).WithPublisher(spy)

	pID := "prod-evt-" + randomID()
	err := service.Add(ctx, pID, "Initial Name", "desc", 1000, "USD", "thumb.jpg")
	is.NoErr(err)

	// Update name and price
	err = service.UpdateProduct(ctx, pID, "Updated Name", "desc", 1500, "USD", "thumb.jpg")
	is.NoErr(err)

	var nameEvt *integration.ProductNameChanged
	var priceEvt *integration.ProductPriceChanged
	for _, e := range spy.events {
		switch evt := e.(type) {
		case integration.ProductNameChanged:
			if evt.ProductID == pID {
				nameEvt = &evt
			}
		case integration.ProductPriceChanged:
			if evt.ProductID == pID {
				priceEvt = &evt
			}
		}
	}

	is.True(nameEvt != nil)
	is.Equal(nameEvt.NewName, "Updated Name")
	is.Equal(nameEvt.VariantID, "var-"+pID)

	is.True(priceEvt != nil)
	is.Equal(priceEvt.PriceAmount, int64(1500))
	is.Equal(priceEvt.PriceCurrency, "USD")
	is.Equal(priceEvt.VariantID, "var-"+pID)
}

func TestProductService_PublishesEventsOnUpdateVariant(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()

	spy := &eventSpy{}
	service := app.NewProductService(storage).WithPublisher(spy)

	pID := "prod-var-evt-" + randomID()
	err := service.AddVariantProduct(ctx, pID, "T-Shirt", "desc", "USD", "thumb.jpg",
		[]app.OptionTypeInput{
			{Name: "Size", Values: []string{"S", "M"}},
		},
		[]app.VariantInput{
			{ID: "var-s-" + pID, SKU: "SKU-S", Price: 2000, Stock: 10, Options: map[string]string{"Size": "S"}},
			{ID: "var-m-" + pID, SKU: "SKU-M", Price: 2200, Stock: 10, Options: map[string]string{"Size": "M"}},
		},
	)
	is.NoErr(err)

	// Clear spy events from creation
	spy.events = nil

	// Update variant price
	err = service.UpdateVariant(ctx, "var-s-"+pID, "SKU-S", "thumb.jpg", 2500, "USD", 10)
	is.NoErr(err)

	var priceEvt *integration.ProductPriceChanged
	for _, e := range spy.events {
		if evt, ok := e.(integration.ProductPriceChanged); ok && evt.VariantID == "var-s-"+pID {
			priceEvt = &evt
		}
	}

	is.True(priceEvt != nil)
	is.Equal(priceEvt.PriceAmount, int64(2500))
	is.Equal(priceEvt.ProductID, pID)

	// Updating product name should emit ProductNameChanged with formatted variant label
	spy.events = nil
	err = service.UpdateProduct(ctx, pID, "Cool T-Shirt", "desc", 2000, "USD", "thumb.jpg")
	is.NoErr(err)

	var names []integration.ProductNameChanged
	for _, e := range spy.events {
		if evt, ok := e.(integration.ProductNameChanged); ok && evt.ProductID == pID {
			names = append(names, evt)
		}
	}

	is.Equal(len(names), 2)
	// One should be for Size S ("Cool T-Shirt — S") and one for Size M ("Cool T-Shirt — M")
	foundS := false
	foundM := false
	for _, n := range names {
		if n.VariantID == "var-s-"+pID && n.NewName == "Cool T-Shirt — S" {
			foundS = true
		}
		if n.VariantID == "var-m-"+pID && n.NewName == "Cool T-Shirt — M" {
			foundM = true
		}
	}
	is.True(foundS)
	is.True(foundM)
}
