package adapter

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/search/app"
	"github.com/bkielbasa/go-ecommerce/backend/search/domain"
)

// docKey is the composite primary key the in-memory store uses. The
// postgres adapter relies on a real (kind, id) primary key; we mirror that
// here so a test asserting Upsert-then-Upsert-of-same-key replaces the
// row rather than appending a second one.
type docKey struct {
	Kind domain.Kind
	ID   string
}

// InMemory is the test-friendly Storage adapter. Query is a naive
// case-insensitive substring scan over title and body — good enough to
// exercise the service-level wiring (Index → Search hit, Remove → no
// hit, RemoveAllOfKind, Kinds filter). Production goes through the
// postgres adapter's full-text index.
type InMemory struct {
	mu   sync.Mutex
	docs map[docKey]domain.Document
}

// NewInMemory returns an empty InMemory storage instance.
func NewInMemory() *InMemory {
	return &InMemory{docs: map[docKey]domain.Document{}}
}

// Upsert stores (or replaces) the document keyed by (Kind, ID).
func (m *InMemory) Upsert(_ context.Context, doc domain.Document) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.docs[docKey{Kind: doc.Kind(), ID: doc.ID()}] = doc
	return nil
}

// Remove deletes the document keyed by (kind, id). Missing rows are a
// no-op (mirroring postgres' DELETE ... WHERE which simply affects 0 rows).
func (m *InMemory) Remove(_ context.Context, kind domain.Kind, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.docs, docKey{Kind: kind, ID: id})
	return nil
}

// Query runs the naive substring scan. The rank for an in-memory hit is
// 1.0 (the service-level tests only check identity, not ordering); ties
// are broken by id for deterministic output.
func (m *InMemory) Query(_ context.Context, q string, opts app.QueryOptions) ([]app.Hit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	needle := strings.ToLower(strings.TrimSpace(q))
	if needle == "" {
		return nil, nil
	}
	kindFilter := map[domain.Kind]bool{}
	for _, k := range opts.Kinds {
		kindFilter[k] = true
	}

	var hits []app.Hit
	for _, d := range m.docs {
		if len(kindFilter) > 0 && !kindFilter[d.Kind()] {
			continue
		}
		hay := strings.ToLower(d.Title() + "\n" + d.Body())
		if !strings.Contains(hay, needle) {
			continue
		}
		hits = append(hits, app.Hit{Document: d, Rank: 1.0})
	}
	sort.Slice(hits, func(i, j int) bool {
		// Deterministic ordering: by kind, then id.
		if hits[i].Document.Kind() != hits[j].Document.Kind() {
			return hits[i].Document.Kind() < hits[j].Document.Kind()
		}
		return hits[i].Document.ID() < hits[j].Document.ID()
	})
	if opts.Limit > 0 && len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return hits, nil
}

// RemoveAllOfKind drops every document owned by the given producer.
func (m *InMemory) RemoveAllOfKind(_ context.Context, kind domain.Kind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.docs {
		if k.Kind == kind {
			delete(m.docs, k)
		}
	}
	return nil
}
