// Package app holds the search application layer: the Storage port the
// adapters satisfy, the Indexer / Querier roles producers and consumers
// depend on, and the Service that implements both. The service is a thin
// pass-through over Storage with a small amount of input normalisation
// (trim the query string before forwarding); the heavy lifting (full-text
// index, ranking) lives in the postgres adapter.
package app

import (
	"context"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/search/domain"
)

// Storage is the persistence port the search adapters satisfy. Two
// implementations ship today: postgres (production) and in-memory (tests).
type Storage interface {
	// Upsert indexes or replaces the document keyed by (Kind, ID).
	Upsert(ctx context.Context, doc domain.Document) error
	// Remove deletes the document keyed by (Kind, ID). Idempotent — removing
	// a row that does not exist is not an error.
	Remove(ctx context.Context, kind domain.Kind, id string) error
	// Query runs a full-text search and returns matching hits ordered by
	// the storage's relevance score (descending).
	Query(ctx context.Context, q string, opts QueryOptions) ([]Hit, error)
	// RemoveAllOfKind deletes every document of the given kind. Used by the
	// `reindex` CLI to start from a clean slate before walking the producer.
	RemoveAllOfKind(ctx context.Context, kind domain.Kind) error
}

// QueryOptions narrows a search. Kinds, when non-empty, restricts the
// result set to those producer kinds; an empty slice means "all kinds".
// Limit caps the result count (0 falls back to defaultLimit).
type QueryOptions struct {
	Kinds []domain.Kind
	Limit int
}

// defaultLimit is the cap applied when QueryOptions.Limit is non-positive.
// The storefront page renders a flat grid, so an upper bound keeps the
// hydration loop bounded; 50 is more than the visible page would render in
// practice and well below the gridtemplate's render budget.
const defaultLimit = 50

// Hit pairs a Document with its storage-supplied relevance Rank. The
// search context does not define what "rank" means beyond "higher is
// better"; for the postgres adapter it is ts_rank_cd over the document's
// tsvector. Consumers are free to ignore Rank.
type Hit struct {
	Document domain.Document
	Rank     float64
}

// Indexer is the OHS write-side port producers depend on. The productcatalog
// app holds its own copy of this interface shape (so it does not have to
// import search/app); the Service satisfies both.
type Indexer interface {
	Index(ctx context.Context, doc domain.Document) error
	Remove(ctx context.Context, kind domain.Kind, id string) error
}

// Querier is the OHS read-side port consumers (the storefront) depend on.
// Mirrors the Storage.Query signature exactly.
type Querier interface {
	Search(ctx context.Context, q string, opts QueryOptions) ([]Hit, error)
}

// Service implements both Indexer (write side) and Querier (read side)
// against a single Storage. One struct, two roles — production wires the
// same instance into both seams (productcatalog's SearchIndexer slot and
// layout's searchService slot).
type Service struct {
	storage Storage
}

// NewService wires the service against a Storage.
func NewService(storage Storage) *Service {
	return &Service{storage: storage}
}

// Index forwards a translated document to storage. The producer's ACL has
// already validated the document via domain.NewDocument; here we only
// pass through.
func (s *Service) Index(ctx context.Context, doc domain.Document) error {
	return s.storage.Upsert(ctx, doc)
}

// Remove deletes the document keyed by (kind, id) from the index.
func (s *Service) Remove(ctx context.Context, kind domain.Kind, id string) error {
	return s.storage.Remove(ctx, kind, id)
}

// Search runs a full-text query against the storage adapter. The query
// string is trimmed; an empty (or whitespace-only) query returns no hits
// without dialling storage (the postgres tsquery would treat it as a
// catch-all match, which is not the intent at the storefront).
func (s *Service) Search(ctx context.Context, q string, opts QueryOptions) ([]Hit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = defaultLimit
	}
	return s.storage.Query(ctx, q, opts)
}

// RemoveAllOfKind is the bulk-clear used by the reindex CLI. It is exposed
// directly (rather than through a "drop everything" sentinel) so producers
// can rebuild their own kind without touching documents owned by other
// producers.
func (s *Service) RemoveAllOfKind(ctx context.Context, kind domain.Kind) error {
	return s.storage.RemoveAllOfKind(ctx, kind)
}
