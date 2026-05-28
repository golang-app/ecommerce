// Package domain holds the search bounded context's published language.
//
// The search context is an Open Host Service (OHS): it publishes a single
// value object — Document — together with a Kind discriminator. Every
// producer that wants to be searchable (productcatalog today; blog and faq
// tomorrow) translates its own domain into a Document via a thin
// Anti-Corruption Layer on the producer side. The search context itself does
// not know about products, blog posts, or anything else — it only knows
// about documents.
//
// Hits are uniquely identified by the pair (Kind, ID). The same id may
// appear under different kinds (a product and a faq about it can share an
// id), and storage adapters key on the pair, not the id alone.
//
// Adding a new producer requires no change here: a producer chooses a new
// Kind value (any non-empty string), translates its records to Documents and
// calls Indexer.Index. The set of known kinds is intentionally NOT a closed
// enum at the type level — it is open by design (that is the OHS pattern).
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Kind discriminates documents by the producer that owns them. The search
// context exposes one constant per producer it has been told about (today:
// product); future producers can add new constants on their side without
// changing search.
type Kind string

// KindProduct is the kind used by the productcatalog producer. It is the
// only kind shipped with the initial cut; blog/faq are deliberately out of
// scope but the architecture accepts new string-valued kinds without
// touching this package.
const KindProduct Kind = "product"

// ErrInvalidDocument is returned by NewDocument when any input fails
// validation (empty kind/id/title; whitespace-only body trimming to empty
// would be allowed — body is informational, title is the primary signal).
var ErrInvalidDocument = errors.New("invalid document")

// Document is the published language of the search OHS. Producers translate
// to it; consumers (the storefront) consume it. The fields are unexported
// so callers cannot smuggle in a half-constructed value; use NewDocument
// (validating) or RebuildDocument (hydration from storage).
type Document struct {
	kind      Kind
	id        string
	title     string
	body      string
	url       string
	tags      []string
	meta      map[string]string
	updatedAt time.Time
}

// NewDocument validates inputs and returns a freshly built Document. tags
// and meta are normalised to non-nil empty containers so callers (templates,
// rebuild helpers) can always range over them without a nil guard.
func NewDocument(kind Kind, id, title, body, url string, tags []string, meta map[string]string, updatedAt time.Time) (Document, error) {
	if strings.TrimSpace(string(kind)) == "" {
		return Document{}, fmt.Errorf("%w: kind cannot be empty", ErrInvalidDocument)
	}
	if strings.TrimSpace(id) == "" {
		return Document{}, fmt.Errorf("%w: id cannot be empty", ErrInvalidDocument)
	}
	if strings.TrimSpace(title) == "" {
		return Document{}, fmt.Errorf("%w: title cannot be empty", ErrInvalidDocument)
	}
	cleanTags := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		cleanTags = append(cleanTags, t)
	}
	cleanMeta := make(map[string]string, len(meta))
	for k, v := range meta {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cleanMeta[k] = v
	}
	return Document{
		kind:      kind,
		id:        id,
		title:     title,
		body:      strings.TrimSpace(body),
		url:       url,
		tags:      cleanTags,
		meta:      cleanMeta,
		updatedAt: updatedAt,
	}, nil
}

// RebuildDocument reconstructs a Document from a storage row, skipping the
// validation that NewDocument performs (the row has been validated once on
// the way in). tags / meta are normalised to non-nil containers.
func RebuildDocument(kind Kind, id, title, body, url string, tags []string, meta map[string]string, updatedAt time.Time) Document {
	if tags == nil {
		tags = []string{}
	}
	if meta == nil {
		meta = map[string]string{}
	}
	return Document{
		kind:      kind,
		id:        id,
		title:     title,
		body:      body,
		url:       url,
		tags:      tags,
		meta:      meta,
		updatedAt: updatedAt,
	}
}

// Kind returns the producer-owned discriminator.
func (d Document) Kind() Kind { return d.kind }

// ID returns the producer-side id (unique within Kind).
func (d Document) ID() string { return d.id }

// Title is the primary text signal — weighted highest in the storage's
// full-text index (the postgres adapter setweights it with 'A').
func (d Document) Title() string { return d.title }

// Body is the secondary text signal (weight 'B' in postgres).
func (d Document) Body() string { return d.body }

// URL is where the storefront should link the search hit; producers own its
// shape (e.g. "/product/<id>"). The search context does not parse it.
func (d Document) URL() string { return d.url }

// Tags are short labels (e.g. category names) used as the tertiary text
// signal (weight 'C' in postgres) and as filter candidates for future UI.
func (d Document) Tags() []string { return d.tags }

// Meta is arbitrary producer-supplied metadata (e.g. currency, price_minor,
// thumbnail). The search context treats it as opaque key/value strings.
func (d Document) Meta() map[string]string { return d.meta }

// UpdatedAt is the producer-supplied last-modified timestamp; storage stamps
// the row updated_at on every upsert in any case.
func (d Document) UpdatedAt() time.Time { return d.updatedAt }
