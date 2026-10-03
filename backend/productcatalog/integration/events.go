// Package integration holds the productcatalog context's published language:
// the integration events other bounded contexts (e.g. cart) may subscribe to.
package integration

import "time"

// ProductNameChanged is published when a product's name is updated in the catalog.
// Carries the ProductID and the affected VariantID along with the new display name.
type ProductNameChanged struct {
	ProductID string
	VariantID string
	NewName   string
	At        time.Time
}

// EventName returns the wire name used by the bus / outbox subscribers.
func (ProductNameChanged) EventName() string { return "productcatalog.ProductNameChanged" }

// ProductPriceChanged is published when a product or variant price is updated.
// Carries the ProductID, VariantID, and the new price in minor units and currency.
type ProductPriceChanged struct {
	ProductID     string
	VariantID     string
	PriceAmount   int64
	PriceCurrency string
	At            time.Time
}

// EventName returns the wire name used by the bus / outbox subscribers.
func (ProductPriceChanged) EventName() string { return "productcatalog.ProductPriceChanged" }
