// Package domain holds the store bounded context's value objects. A Store
// is an immutable description of one storefront facade — a slug, a name,
// the display currency the storefront prices things in, and the request
// Host header it is reached at. The active store is resolved per request
// (see the layout package's storeMiddleware) and threaded onto the request
// context so every render reads the same one.
package domain

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidStore is returned by NewStore when the supplied parameters
// fail validation (empty fields, bad slug, malformed currency code, etc.).
var ErrInvalidStore = errors.New("invalid store")

// slugPattern enforces the canonical "lowercase letters / digits /
// hyphens" shape so a slug is safe to embed in URLs and DOM ids.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// Store is the value object describing a single storefront facade. The
// fields are intentionally unexported — every read goes through a getter
// so the value object stays immutable from the outside. Stores are
// constructed via NewStore (validates) or RebuildStore (hydration from
// storage).
type Store struct {
	id        string
	slug      string
	name      string
	currency  string
	host      string
	isDefault bool
	position  int
}

// NewStore validates and constructs a fresh Store. The id is supplied by
// the caller (operators want a stable, human-meaningful identifier such
// as "us" or "eu" — the slug doubles as a primary key in the seeds).
func NewStore(id, slug, name, currency, host string, isDefault bool, position int) (Store, error) {
	id = strings.TrimSpace(id)
	slug = strings.TrimSpace(slug)
	name = strings.TrimSpace(name)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	host = strings.TrimSpace(host)

	if id == "" {
		return Store{}, ErrInvalidStore
	}
	if !slugPattern.MatchString(slug) {
		return Store{}, ErrInvalidStore
	}
	if name == "" {
		return Store{}, ErrInvalidStore
	}
	if len(currency) != 3 {
		return Store{}, ErrInvalidStore
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return Store{}, ErrInvalidStore
		}
	}
	if host == "" {
		return Store{}, ErrInvalidStore
	}
	if position < 0 {
		return Store{}, ErrInvalidStore
	}
	return Store{
		id:        id,
		slug:      slug,
		name:      name,
		currency:  currency,
		host:      host,
		isDefault: isDefault,
		position:  position,
	}, nil
}

// RebuildStore is the hydration constructor used by the adapter layer.
// It bypasses validation so adapters can reconstruct exactly what the
// database returned (including legacy rows that pre-date a tightened
// invariant).
func RebuildStore(id, slug, name, currency, host string, isDefault bool, position int) Store {
	return Store{
		id:        id,
		slug:      slug,
		name:      name,
		currency:  currency,
		host:      host,
		isDefault: isDefault,
		position:  position,
	}
}

// ID returns the store's stable identifier.
func (s Store) ID() string { return s.id }

// Slug returns the URL-safe slug.
func (s Store) Slug() string { return s.slug }

// Name returns the human-facing display name.
func (s Store) Name() string { return s.name }

// Currency returns the 3-letter ISO display currency the storefront
// prices things in.
func (s Store) Currency() string { return s.currency }

// Host returns the request Host header this store binds to
// (e.g. "localhost:8080" or "eu.localhost:8080").
func (s Store) Host() string { return s.host }

// IsDefault reports whether this is the fallback store used by
// Service.ResolveByHost when the request Host matches no other row.
// Exactly one store is the default; the postgres adapter enforces it
// with a partial unique index.
func (s Store) IsDefault() bool { return s.isDefault }

// Position is the operator-supplied ordering hint. Lower comes first in
// the footer store switcher / admin list.
func (s Store) Position() int { return s.position }

// IsZero reports whether the receiver is the zero-value Store. Used by
// the layout's storeMiddleware fallback so the renderer can tell that no
// store was resolvable for the request.
func (s Store) IsZero() bool { return s.id == "" }
