// Package domain holds the recommendation context's value objects:
// the five-weight Strategy scorer and the per-product top-N
// recommendation row.
//
// SCORING (STRATEGY PATTERN)
//
// The score for a (seed, candidate) pair is a weighted sum of five
// orthogonal signals, each normalised to [0,1]:
//
//	score = α·co_purchase + β·text + γ·same_category +
//	        δ·attribute_jaccard + ε·price_proximity
//
// The five weights live in a single Weights value object so the
// scoring strategy is one method call away from any caller. The
// weights are admin-tunable at runtime — they are loaded from the
// recommendation_weights table at the start of every refresh — so
// the demo can show how an operator dials up the "people who bought
// this also bought" signal without redeploying. The constructor
// validates that no weight is negative but otherwise accepts any
// magnitude; the absolute scale of the final score is irrelevant —
// only the relative ordering inside one seed's top-N matters.
package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidWeights is returned by NewWeights when any weight is
// negative. The sentinel is wrapped via fmt.Errorf("...: %w", ...) so
// callers can branch on it with errors.Is.
var ErrInvalidWeights = errors.New("recommendation: invalid weights")

// Default weights apply when the persisted singleton row carries the
// stock values (and as the in-process fallback when WeightsStorage
// returns no row). The demo's defaults bias toward co-purchase (a
// behaviour-based signal) while keeping the four content signals as
// a meaningful tie-breaker.
const (
	defaultWeightCoPurchase = 0.40
	defaultWeightText       = 0.20
	defaultWeightCategory   = 0.20
	defaultWeightAttributes = 0.10
	defaultWeightPrice      = 0.10
)

// Weights is the five-coefficient scoring Strategy. The struct is a
// pure value object — no methods mutate it; Score(ScoreInputs) is the
// only behaviour and it is referentially transparent.
type Weights struct {
	CoPurchase float64
	Text       float64
	Category   float64
	Attributes float64
	Price      float64
}

// DefaultWeights returns the canonical defaults used when no admin
// override is persisted. Mirrors the recommendation_weights table's
// column defaults; the two sites are kept in sync deliberately so a
// fresh boot against an empty table behaves identically to a boot
// against the seeded row.
func DefaultWeights() Weights {
	return Weights{
		CoPurchase: defaultWeightCoPurchase,
		Text:       defaultWeightText,
		Category:   defaultWeightCategory,
		Attributes: defaultWeightAttributes,
		Price:      defaultWeightPrice,
	}
}

// NewWeights validates that every weight is non-negative. The sum is
// NOT forced to one — operators can legitimately use weights summing
// to anything, since only the relative ordering of candidates for
// one seed matters. A sum of zero is allowed (every candidate scores
// zero and the storefront falls back to the cold-start path), but
// callers should treat it as a foot-gun: NewWeights does not reject
// the all-zero case, it merely returns the value as-is.
func NewWeights(coPurchase, text, category, attributes, price float64) (Weights, error) {
	if coPurchase < 0 || text < 0 || category < 0 || attributes < 0 || price < 0 {
		return Weights{}, fmt.Errorf("weights must be non-negative: %w", ErrInvalidWeights)
	}
	return Weights{
		CoPurchase: coPurchase,
		Text:       text,
		Category:   category,
		Attributes: attributes,
		Price:      price,
	}, nil
}

// Sum returns the arithmetic sum of every weight. Useful to the
// admin UI: a zero sum is rendered with a warning because every
// candidate then scores zero and the storefront silently falls back
// to "popular in category". A non-zero sum is otherwise fine — the
// magnitude is irrelevant to ranking.
func (w Weights) Sum() float64 {
	return w.CoPurchase + w.Text + w.Category + w.Attributes + w.Price
}

// ScoreInputs is the per-pair feature vector the refresher passes
// into Score. Every field is expected to be in [0,1]; values outside
// that range are not rejected (they distort the weighted sum but do
// not error) — keeping the contract a documentation concern rather
// than a runtime check lets tests poke specific values without
// fighting validation.
type ScoreInputs struct {
	// CoPurchaseNorm is the count of how often the pair appeared in
	// the same paid order, normalised by the seed's max pair count
	// so popular products don't drown out niche ones.
	CoPurchaseNorm float64
	// TextSimilarity is a cheap in-memory token Jaccard between the
	// seed's and candidate's name+description. Postgres FTS is used
	// to fetch CANDIDATES (a tighter pool than "every other
	// product"); the score itself does not re-run FTS per pair.
	TextSimilarity float64
	// SameCategory is 1.0 when the two products share their first
	// category, 0.0 otherwise. A coarse-but-stable signal that is
	// cheap to compute from the in-memory ProductSummary.
	SameCategory float64
	// AttributeJaccard is the Jaccard similarity over the products'
	// (key=value) attribute pairs. Captures "same brand & material"
	// without depending on the catalogue's full attribute schema.
	AttributeJaccard float64
	// PriceProximity is 1 minus the relative price gap, so two
	// products with identical prices score 1 and an order-of-magnitude
	// gap scores ~0. Clamps at zero for outliers.
	PriceProximity float64
}

// Score is the dimensionless weighted-sum output. The type is named
// rather than a bare float64 so APIs that move it around (e.g. the
// storage Upsert) keep the units obvious at the call site.
type Score float64

// Score returns the weighted sum α·CP + β·Text + γ·Cat + δ·Attr +
// ε·Price. Pure, deterministic, no allocation; the refresher invokes
// it once per candidate per seed.
func (w Weights) Score(in ScoreInputs) Score {
	return Score(
		w.CoPurchase*in.CoPurchaseNorm +
			w.Text*in.TextSimilarity +
			w.Category*in.SameCategory +
			w.Attributes*in.AttributeJaccard +
			w.Price*in.PriceProximity,
	)
}
