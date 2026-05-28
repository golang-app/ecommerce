package app

import "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"

// Range is a numeric filter bound. A nil Min or Max means that side is
// unbounded.
type Range struct {
	Min, Max *float64
}

// ProductQuery describes a listing-page filter: an optional category (by slug),
// numeric attribute ranges and enum attribute selections. The maps are keyed by
// attribute-type id. Search, when non-empty, applies a case-insensitive
// substring match across the product name and description.
type ProductQuery struct {
	CategorySlug   string
	NumericRanges  map[string]Range
	EnumSelections map[string][]string
	Search         string
}

// AttributeAssignment is a single product attribute value to persist. Num is
// set (and Text empty) for numeric types; Text is set (and Num nil) for enum
// types.
type AttributeAssignment struct {
	TypeID string
	Num    *float64
	Text   string
}

// Facet describes the available filter options for one attribute type among
// the in-scope products. For a numeric type Min/Max bound the available range;
// for an enum type Values lists the distinct available values, sorted.
type Facet struct {
	Type   domain.AttributeType
	Min    *float64
	Max    *float64
	Values []string
}
