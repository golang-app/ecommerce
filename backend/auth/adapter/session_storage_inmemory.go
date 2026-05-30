package adapter

import (
	"context"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

// inMemorySessionStorage is the in-memory session backing used by short
// tests. A single map is shared across all callers (the test wiring
// hands the same struct to both the customer and admin services so
// concurrent customer/admin sessions in the same test process behave
// the way they do in postgres). The kind filter is enforced per
// instance via the lightweight wrapper below.
type inMemorySessionStorage struct {
	mx    sync.Mutex
	store map[string]sessionEntry
}

type sessionEntry struct {
	session *domain.Session
	kind    string
}

func NewInMemorySessionStorage() *inMemorySessionStorage {
	return &inMemorySessionStorage{store: make(map[string]sessionEntry)}
}

// CustomerScope returns a SessStorage that writes and reads only
// customer-kind sessions backed by this store. It exists so the two
// services (CustomerAuth + AdminAuth) can share one underlying map
// without leaking each other's tokens.
func (p *inMemorySessionStorage) CustomerScope() *scopedInMemorySessionStorage {
	return &scopedInMemorySessionStorage{parent: p, kind: PrincipalKindCustomer}
}

// AdminScope mirrors CustomerScope for admin sessions.
func (p *inMemorySessionStorage) AdminScope() *scopedInMemorySessionStorage {
	return &scopedInMemorySessionStorage{parent: p, kind: PrincipalKindAdmin}
}

// Store on the bare in-memory storage defaults to customer kind so
// existing callers (notably the short auth_test bootstrap that hands
// the bare store to NewAuth) keep working without ceremony.
func (p *inMemorySessionStorage) Store(ctx context.Context, session *domain.Session) error {
	p.mx.Lock()
	defer p.mx.Unlock()
	p.store[session.ID()] = sessionEntry{session: session, kind: PrincipalKindCustomer}
	return nil
}

func (p *inMemorySessionStorage) Find(ctx context.Context, token string) (*domain.Session, error) {
	p.mx.Lock()
	defer p.mx.Unlock()
	entry, ok := p.store[token]
	if !ok || entry.kind != PrincipalKindCustomer {
		return nil, domain.ErrSessionNotFound
	}
	return entry.session, nil
}

type scopedInMemorySessionStorage struct {
	parent *inMemorySessionStorage
	kind   string
}

func (p *scopedInMemorySessionStorage) Store(ctx context.Context, session *domain.Session) error {
	p.parent.mx.Lock()
	defer p.parent.mx.Unlock()
	p.parent.store[session.ID()] = sessionEntry{session: session, kind: p.kind}
	return nil
}

func (p *scopedInMemorySessionStorage) Find(ctx context.Context, token string) (*domain.Session, error) {
	p.parent.mx.Lock()
	defer p.parent.mx.Unlock()
	entry, ok := p.parent.store[token]
	if !ok || entry.kind != p.kind {
		return nil, domain.ErrSessionNotFound
	}
	return entry.session, nil
}
