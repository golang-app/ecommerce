package app

import (
	"context"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// defaultTopNPage is how many recommendations the storefront's "you
// might also like" section shows. The refresher writes up to ten
// rows per seed (so a future page can show more without a re-run);
// the storefront reads six by default.
const defaultTopNPage = 6

// Service is the only port the storefront talks to — every call to
// "give me the recs for product X" goes through Recommendations. The
// cold-start fallback (no rows in storage yet) is hidden inside the
// service; from the storefront's point of view the recommendation
// section either has products or does not.
type Service struct {
	storage  Storage
	catalog  CatalogReader
	weights  WeightsStorage
	topNPage int
}

// NewService wires the application facade. WeightsStorage is taken
// so callers can read/write the admin-tunable weights through one
// surface; the admin handler talks to this method (Weights / Set).
func NewService(storage Storage, catalog CatalogReader, weights WeightsStorage) *Service {
	return &Service{
		storage:  storage,
		catalog:  catalog,
		weights:  weights,
		topNPage: defaultTopNPage,
	}
}

// WithTopNPage overrides how many products the storefront reads.
// Tests use it to assert "exactly N" without seeding ten products.
func (s *Service) WithTopNPage(n int) *Service {
	if n > 0 {
		s.topNPage = n
	}
	return s
}

// Recommendations is the SOLE storefront entry point. It returns a
// hydrated slice of ProductSummary the template can render:
//
//  1. Try Storage.TopN. If non-empty: hydrate each recommended id
//     via CatalogReader.ProductByID (best-effort: skip stale ids).
//  2. Cold-start fallback. If TopN returns no rows OR the seed has
//     never been refreshed yet, look up the seed itself to learn
//     its category and ask CatalogReader.ProductsInCategory for up
//     to `topNPage+1` peers; drop the seed from the result; return
//     the first `topNPage`.
//
// All errors are wrapped with %w so callers can branch with
// errors.Is; the storefront handler turns any non-nil error into an
// "omit the section" decision so a flaky recommendation context
// never breaks the product detail page.
func (s *Service) Recommendations(ctx context.Context, productID string) ([]domain.ProductSummary, error) {
	if productID == "" {
		return nil, nil
	}
	recs, err := s.storage.TopN(ctx, productID, s.topNPage)
	if err != nil {
		return nil, fmt.Errorf("recommendation: load top-n: %w", err)
	}
	if len(recs) > 0 {
		return s.hydrate(ctx, recs, productID)
	}
	return s.fallback(ctx, productID)
}

// hydrate translates the persisted Recommendation rows into the
// ProductSummary values the template wants to render. A stale row
// (the recommended product was deleted from the catalogue after the
// last refresh) is silently dropped — better than 500ing the
// product page on a transient inconsistency.
func (s *Service) hydrate(ctx context.Context, recs []domain.Recommendation, seed string) ([]domain.ProductSummary, error) {
	out := make([]domain.ProductSummary, 0, len(recs))
	for _, r := range recs {
		if r.RecommendedID == seed {
			// Defensive: the refresher already filters self-pairs
			// before writing, but a manual UpsertTopN from a
			// future admin tool could slip one through. Drop it.
			continue
		}
		p, ok, err := s.catalog.ProductByID(ctx, r.RecommendedID)
		if err != nil {
			return nil, fmt.Errorf("recommendation: hydrate %s: %w", r.RecommendedID, err)
		}
		if !ok {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// fallback implements the cold-start path: the seed has no
// materialised top-N yet, so we serve "popular in category" via the
// catalogue ACL. The seed itself is filtered out of the returned
// list — the storefront never wants to recommend a product back to
// itself. We ask for topNPage+1 to keep the result at topNPage even
// when the seed appears among the category's first products.
func (s *Service) fallback(ctx context.Context, productID string) ([]domain.ProductSummary, error) {
	seed, ok, err := s.catalog.ProductByID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("recommendation: load seed: %w", err)
	}
	if !ok || seed.CategoryID == "" {
		// Either the seed itself is missing or it has no category
		// — we have nothing principled to fall back on. Return an
		// empty slice; the storefront then renders no section.
		return nil, nil
	}
	peers, err := s.catalog.ProductsInCategory(ctx, seed.CategoryID, s.topNPage+1)
	if err != nil {
		return nil, fmt.Errorf("recommendation: cold-start fallback: %w", err)
	}
	out := make([]domain.ProductSummary, 0, len(peers))
	for _, p := range peers {
		if p.ID == productID {
			continue
		}
		out = append(out, p)
		if len(out) >= s.topNPage {
			break
		}
	}
	return out, nil
}

// Weights returns the persisted weights so the admin page can show
// the current values in the form inputs. A missing row falls back
// to defaults inside the storage layer.
func (s *Service) Weights(ctx context.Context) (domain.Weights, error) {
	return s.weights.Get(ctx)
}

// SetWeights validates non-negative weights via the domain
// constructor and persists them. The admin handler calls this
// directly; the refresher will pick up the new value at the start
// of its next tick.
func (s *Service) SetWeights(ctx context.Context, w domain.Weights) error {
	validated, err := domain.NewWeights(w.CoPurchase, w.Text, w.Category, w.Attributes, w.Price)
	if err != nil {
		return err
	}
	return s.weights.Set(ctx, validated)
}

// LastRefresh returns the latest computed_at across every seed (the
// admin dashboard renders it as "last refreshed at …"). ok=false
// means the refresher has not yet completed a tick.
func (s *Service) LastRefresh(ctx context.Context) (time.Time, bool, error) {
	return s.storage.OverallLastRefresh(ctx)
}
