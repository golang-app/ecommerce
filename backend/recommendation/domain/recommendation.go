package domain

// ProductSummary is the recommendation context's local copy of the
// fields it needs about a product. It is intentionally NOT the
// productcatalog domain type: the recommendation context never
// imports productcatalog's domain package. The catalog ACL adapter
// (recommendation/adapter/catalog_acl.go) translates productcatalog's
// Product into this summary on the way in, so the context boundary
// stays one-way and the recommendation context owns its own
// vocabulary.
//
// Attributes is a flat key/value map (NOT productcatalog's
// AttributeValue slice) because the only operation the scorer
// performs on it is a Jaccard intersection over (key=value) pairs —
// a map is the cheapest shape for that. The ACL flattens whatever
// the catalog exposes into this map before crossing the boundary.
type ProductSummary struct {
	ID          string
	CategoryID  string
	Name        string
	Description string
	PriceMinor  int64
	Attributes  map[string]string
}

// Recommendation is one row in a seed product's materialised top-N.
// The refresher writes these via Storage.UpsertTopN; the storefront
// reads them via Storage.TopN. Position is 0..N-1 inside the seed's
// own list so the storefront can render the cards in the order the
// refresher decided without an extra sort. Score is included so the
// admin debug page (if/when added) can show why a given candidate
// was picked.
type Recommendation struct {
	ProductID     string
	RecommendedID string
	Score         Score
	Position      int
}

// Pair models an unordered pair of product ids — the natural key
// for the co-purchase counter the refresher builds from the order
// history. Always construct via NewPair so the two ids are sorted
// before being stored in the struct; that way (a,b) and (b,a)
// collapse to the same map key without callers having to remember.
type Pair struct {
	A, B string
}

// NewPair returns a Pair with the two ids sorted lexically. Calling
// it with two equal ids returns the same value back — the caller is
// responsible for filtering out the trivial self-pair (a product
// "co-purchased with itself" never adds information). Pairs whose
// ids match are still legal Pair values; the order-history ACL
// filters them out before they reach the scorer.
func NewPair(a, b string) Pair {
	if a <= b {
		return Pair{A: a, B: b}
	}
	return Pair{A: b, B: a}
}

// Other returns the id in the pair that is NOT id. When id is not
// part of the pair (the caller's bug), Other returns the empty
// string — the refresher treats that as "skip this pair" rather
// than panicking, keeping the call site terse.
func (p Pair) Other(id string) string {
	switch id {
	case p.A:
		return p.B
	case p.B:
		return p.A
	}
	return ""
}
