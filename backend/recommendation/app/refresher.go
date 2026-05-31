package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// Refresher defaults — these are the knobs the composition root may
// override with the WithXxx setters before kicking off RunForever.
const (
	defaultTopNStore    = 10  // how many rows to persist per seed
	defaultCandidatesK  = 20  // how many candidates per generator
	defaultWindowDays   = 90  // co-purchase lookback window
	defaultRefreshEvery = 15 * time.Minute
	// minTokenLen filters out very short tokens (articles,
	// prepositions, "a", "of") from the in-memory Jaccard. Three
	// characters is a balance between dropping noise and keeping
	// enough useful tokens on a short product description.
	minTokenLen = 3
)

// Logger is the minimal log seam the refresher uses for non-fatal
// warnings and per-tick duration. The default is a no-op logger so
// the refresher can be instantiated without a logrus dependency
// (tests do this).
type Logger interface {
	Warnf(format string, args ...any)
	Infof(format string, args ...any)
}

// nopLogger discards every message; used by NewRefresher when no
// explicit logger is wired.
type nopLogger struct{}

func (nopLogger) Warnf(string, ...any) {}
func (nopLogger) Infof(string, ...any) {}

// Refresher is the timer-driven goroutine that materialises the
// top-N read model. It pulls co-purchase pairs and the current
// weights ONCE per tick, then walks every product id and writes its
// top-N list under a transaction. The score per (seed, candidate)
// is the five-weight Strategy from domain/score.go.
//
// CONTROL FLOW
//
//	RunForever(ctx, interval) ticks every `interval`; each tick
//	calls RefreshAll(ctx). RefreshAll loads the weights and the
//	co-purchase map up front (one query each), then iterates
//	AllProductIDs and calls refreshOne per seed. RunForever
//	respects ctx.Done — when the context is cancelled the ticker
//	is stopped and the loop returns.
//
// CANDIDATE GENERATION (refreshOne)
//
//	The candidate pool for a seed is the union of three sources:
//	  1. Co-purchase partners — every id that has appeared in the
//	     same paid order as the seed during the lookback window.
//	  2. ProductsInCategory(seed.CategoryID, K) — same-category
//	     peers (catches "looks like the seed but never co-purchased").
//	  3. SimilarByText(seed.ID, K) — Postgres FTS against the
//	     catalogue table (catches text-heavy similarity that the
//	     in-memory Jaccard would miss because the seed and
//	     candidate share no tokens beyond stopwords).
//
//	The three sources are deduplicated by id and the seed itself
//	is removed before scoring.
type Refresher struct {
	catalog      CatalogReader
	history      OrderHistoryReader
	storage      Storage
	weights      WeightsStorage
	logger       Logger
	now          func() time.Time
	topNStore    int
	candidatesK  int
	windowDays   int
	refreshEvery time.Duration
}

// NewRefresher wires the refresher against its dependencies. The
// defaults match the demo's expectations; production composition
// can dial them via the chainable With… setters.
func NewRefresher(catalog CatalogReader, history OrderHistoryReader, storage Storage, weights WeightsStorage) *Refresher {
	return &Refresher{
		catalog:      catalog,
		history:      history,
		storage:      storage,
		weights:      weights,
		logger:       nopLogger{},
		now:          func() time.Time { return time.Now().UTC() },
		topNStore:    defaultTopNStore,
		candidatesK:  defaultCandidatesK,
		windowDays:   defaultWindowDays,
		refreshEvery: defaultRefreshEvery,
	}
}

// WithLogger installs a non-nil logger.
func (r *Refresher) WithLogger(l Logger) *Refresher {
	if l != nil {
		r.logger = l
	}
	return r
}

// WithClock overrides the time source so tests can pin
// "since now-N days" deterministically.
func (r *Refresher) WithClock(now func() time.Time) *Refresher {
	if now != nil {
		r.now = now
	}
	return r
}

// WithTopN overrides the per-seed row count the refresher writes.
func (r *Refresher) WithTopN(n int) *Refresher {
	if n > 0 {
		r.topNStore = n
	}
	return r
}

// WithCandidatesK overrides how many candidates each of the three
// generators (co-purchase / category / text) contributes before the
// scorer reduces them to the final top-N.
func (r *Refresher) WithCandidatesK(k int) *Refresher {
	if k > 0 {
		r.candidatesK = k
	}
	return r
}

// WithWindow sets the co-purchase lookback in days. A zero or
// negative value falls back to defaultWindowDays.
func (r *Refresher) WithWindow(days int) *Refresher {
	if days > 0 {
		r.windowDays = days
	}
	return r
}

// RefreshAll runs ONE refresh pass over every product id in the
// catalogue. Co-purchase pairs and the weights are loaded once at
// the top so the per-seed loop is O(products × candidates) without
// re-querying. A per-seed failure is logged at Warn but does not
// abort the pass — partial refreshes are preferable to "the
// refresher crashes and the storefront stays at yesterday's top-N
// forever".
func (r *Refresher) RefreshAll(ctx context.Context) error {
	start := r.now()

	weights, err := r.weights.Get(ctx)
	if err != nil {
		return fmt.Errorf("recommendation refresher: load weights: %w", err)
	}

	since := start.Add(-time.Duration(r.windowDays) * 24 * time.Hour)
	pairs, err := r.history.CoPurchasePairs(ctx, since)
	if err != nil {
		return fmt.Errorf("recommendation refresher: load co-purchase pairs: %w", err)
	}

	ids, err := r.catalog.AllProductIDs(ctx)
	if err != nil {
		return fmt.Errorf("recommendation refresher: list product ids: %w", err)
	}

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			// Cooperative cancellation: the next tick (or an
			// admin "refresh now") will resume from a fresh
			// snapshot. Returning nil so RunForever's caller
			// treats the cancel as a graceful exit.
			return nil
		}
		if err := r.refreshOne(ctx, id, pairs, weights); err != nil {
			r.logger.Warnf("recommendation refresher: seed %s: %v", id, err)
		}
	}
	r.logger.Infof("recommendation refresher: pass completed in %s (products=%d)", r.now().Sub(start), len(ids))
	return nil
}

// RefreshProduct runs the refresh pass for ONE seed. The admin
// "refresh now" button could call it for a specific product; for
// now the button kicks the whole pass instead. Exposed on the
// service surface so a future targeted-refresh feature stays a
// single-line wiring change.
func (r *Refresher) RefreshProduct(ctx context.Context, productID string) error {
	weights, err := r.weights.Get(ctx)
	if err != nil {
		return fmt.Errorf("recommendation refresher: load weights: %w", err)
	}
	since := r.now().Add(-time.Duration(r.windowDays) * 24 * time.Hour)
	pairs, err := r.history.CoPurchasePairs(ctx, since)
	if err != nil {
		return fmt.Errorf("recommendation refresher: load co-purchase pairs: %w", err)
	}
	return r.refreshOne(ctx, productID, pairs, weights)
}

// RunForever drives RefreshAll on a ticker. The first refresh runs
// IMMEDIATELY (not after `interval`) so a fresh boot has a populated
// top-N within a few seconds; subsequent refreshes follow the
// cadence. A non-positive interval falls back to the default. The
// loop exits when ctx is cancelled.
func (r *Refresher) RunForever(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = r.refreshEvery
	}
	// First refresh runs synchronously here (well, "synchronously
	// inside the goroutine the composition root spawned"). A panic
	// or hard error is logged; the ticker still starts so the next
	// tick gets another shot.
	if err := r.RefreshAll(ctx); err != nil {
		r.logger.Warnf("recommendation refresher: initial pass: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.logger.Infof("recommendation refresher: stopping (context cancelled)")
			return
		case <-ticker.C:
			if err := r.RefreshAll(ctx); err != nil {
				r.logger.Warnf("recommendation refresher: pass: %v", err)
			}
		}
	}
}

// refreshOne computes and persists the top-N for a single seed. The
// flow is:
//
//  1. Load the seed's ProductSummary (skip if missing — a deleted
//     product should not leave stale rows pointing at it).
//  2. Build the candidate set via candidatesFor.
//  3. Hydrate each candidate's ProductSummary (skip missing).
//  4. Score every candidate via the weights Strategy.
//  5. Sort by score desc, take the first topNStore, assign positions.
//  6. UpsertTopN — atomic replace of the previous list.
func (r *Refresher) refreshOne(ctx context.Context, seedID string, pairs map[domain.Pair]int, weights domain.Weights) error {
	seed, ok, err := r.catalog.ProductByID(ctx, seedID)
	if err != nil {
		return fmt.Errorf("load seed: %w", err)
	}
	if !ok {
		return nil
	}

	candidateIDs, maxCoPurchase := r.candidatesFor(ctx, seed, pairs)
	if len(candidateIDs) == 0 {
		// Nothing to score; leave any prior rows in place. A
		// future "drop stale rows" pass could clear them, but for
		// the demo the cold-start fallback already handles the
		// empty case at read time.
		return nil
	}

	scored := make([]domain.Recommendation, 0, len(candidateIDs))
	for cid := range candidateIDs {
		candidate, found, cErr := r.catalog.ProductByID(ctx, cid)
		if cErr != nil {
			r.logger.Warnf("recommendation refresher: load candidate %s: %v", cid, cErr)
			continue
		}
		if !found {
			continue
		}
		inputs := computeInputs(seed, candidate, pairs, maxCoPurchase)
		s := weights.Score(inputs)
		scored = append(scored, domain.Recommendation{
			ProductID:     seedID,
			RecommendedID: cid,
			Score:         s,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > r.topNStore {
		scored = scored[:r.topNStore]
	}
	for i := range scored {
		scored[i].Position = i
	}
	return r.storage.UpsertTopN(ctx, seedID, scored)
}

// candidatesFor builds the deduplicated candidate set and computes
// the seed's max co-purchase count (used to normalise the
// per-candidate CoPurchaseNorm into [0,1]). Three sources contribute:
// co-purchase partners (frequency-weighted), same-category peers,
// and FTS-similar products.
func (r *Refresher) candidatesFor(ctx context.Context, seed domain.ProductSummary, pairs map[domain.Pair]int) (map[string]struct{}, int) {
	candidates := map[string]struct{}{}
	maxCount := 0

	// Source 1: co-purchase partners. We walk the pair map once
	// here (cheap — the demo's catalogue is ~50 products) and
	// extract every pair the seed participates in. The seed itself
	// is filtered out via Pair.Other; pairs unrelated to the seed
	// return "" and are skipped.
	for pair, count := range pairs {
		other := pair.Other(seed.ID)
		if other == "" {
			continue
		}
		candidates[other] = struct{}{}
		if count > maxCount {
			maxCount = count
		}
	}

	// Source 2: same-category peers.
	if seed.CategoryID != "" {
		peers, err := r.catalog.ProductsInCategory(ctx, seed.CategoryID, r.candidatesK)
		if err != nil {
			r.logger.Warnf("recommendation refresher: load category peers for %s: %v", seed.ID, err)
		} else {
			for _, p := range peers {
				if p.ID == seed.ID {
					continue
				}
				candidates[p.ID] = struct{}{}
			}
		}
	}

	// Source 3: text-similar via FTS.
	similar, err := r.catalog.SimilarByText(ctx, seed.ID, r.candidatesK)
	if err != nil {
		r.logger.Warnf("recommendation refresher: load text-similar for %s: %v", seed.ID, err)
	} else {
		for _, p := range similar {
			if p.ID == seed.ID {
				continue
			}
			candidates[p.ID] = struct{}{}
		}
	}

	return candidates, maxCount
}

// computeInputs is the pure feature-extraction function the
// refresher feeds into the Weights.Score Strategy. Every output
// dimension is in [0,1] by construction.
func computeInputs(seed, candidate domain.ProductSummary, pairs map[domain.Pair]int, maxCoPurchase int) domain.ScoreInputs {
	in := domain.ScoreInputs{}

	if maxCoPurchase > 0 {
		count := pairs[domain.NewPair(seed.ID, candidate.ID)]
		in.CoPurchaseNorm = float64(count) / float64(maxCoPurchase)
	}

	in.TextSimilarity = tokenJaccard(
		seed.Name+" "+seed.Description,
		candidate.Name+" "+candidate.Description,
	)

	if seed.CategoryID != "" && seed.CategoryID == candidate.CategoryID {
		in.SameCategory = 1.0
	}

	in.AttributeJaccard = attributeJaccard(seed.Attributes, candidate.Attributes)
	in.PriceProximity = priceProximity(seed.PriceMinor, candidate.PriceMinor)

	return in
}

// tokenJaccard normalises both inputs (lowercase, split on
// non-letters, drop tokens shorter than minTokenLen) and returns
// |A ∩ B| / |A ∪ B|. Identical token sets score 1; disjoint sets
// score 0; both inputs empty score 0 (the storefront does not want
// to recommend two no-text products as "highly similar by text").
func tokenJaccard(a, b string) float64 {
	setA := tokenise(a)
	setB := tokenise(b)
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	inter := 0
	for tok := range setA {
		if _, ok := setB[tok]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// tokenise lowercases the input and splits on any non-letter rune
// (digits are dropped — they rarely carry similarity signal in
// product names and they explode the token set with "v2", "model
// 3", etc.). Tokens shorter than minTokenLen are filtered out so
// stopwords ("a", "of", "to") don't dominate the Jaccard score.
func tokenise(s string) map[string]struct{} {
	out := map[string]struct{}{}
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	for _, f := range fields {
		if len(f) >= minTokenLen {
			out[f] = struct{}{}
		}
	}
	return out
}

// attributeJaccard treats each (key=value) pair as a single set
// member so "brand=acme" and "brand=other" count as different
// members (not as "shared brand key"). Identical attribute maps
// score 1; both empty scores 0.
func attributeJaccard(a, b map[string]string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := map[string]struct{}{}
	for k, v := range a {
		setA[k+"="+v] = struct{}{}
	}
	setB := map[string]struct{}{}
	for k, v := range b {
		setB[k+"="+v] = struct{}{}
	}
	inter := 0
	for kv := range setA {
		if _, ok := setB[kv]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// priceProximity returns 1 − |Δp| / max(p₁,p₂), clamped to [0,1].
// Two identical prices score 1; an order-of-magnitude gap scores
// ~0.1; the function returns 1 when both inputs are zero (two
// zero-priced products are trivially close).
func priceProximity(a, b int64) float64 {
	if a == 0 && b == 0 {
		return 1
	}
	maxP := a
	if b > maxP {
		maxP = b
	}
	if maxP <= 0 {
		return 0
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	v := 1 - float64(diff)/float64(maxP)
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	// math.IsNaN guard: defensive; the inputs above can't produce
	// NaN today, but keeping the check makes the function safe to
	// reuse with future inputs.
	if math.IsNaN(v) {
		return 0
	}
	return v
}
