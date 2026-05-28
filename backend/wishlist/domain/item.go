// Package domain holds the wishlist bounded-context value object: an Item
// is a single (customer, variant) bookmark with the timestamp it was saved.
// Wishlists are keyed by variant id rather than product id so that each
// colour / size of a variant product is independently saveable.
package domain

import "time"

// Item is the value object persisted in wishlist_item. A wishlist is just
// the unordered set of these for a given customer; the application layer
// presents them newest-first by addedAt.
type Item struct {
	customerID string
	variantID  string
	addedAt    time.Time
}

// Rebuild reconstructs an Item from a storage row. The wishlist domain has
// no construction-time validation (the customer / variant ids are opaque to
// it) so callers — including the application service — go through this
// constructor too.
func Rebuild(customerID, variantID string, addedAt time.Time) Item {
	return Item{customerID: customerID, variantID: variantID, addedAt: addedAt}
}

func (i Item) CustomerID() string { return i.customerID }
func (i Item) VariantID() string  { return i.variantID }
func (i Item) AddedAt() time.Time { return i.addedAt }
