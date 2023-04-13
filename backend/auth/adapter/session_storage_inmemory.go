package adapter

import (
	"context"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
)

type inMemorySessionStorage struct {
	mx    sync.Mutex
	store map[string]*domain.Session
}

func NewInMemorySessionStorage() *inMemorySessionStorage {
	return &inMemorySessionStorage{
		store: make(map[string]*domain.Session),
	}
}

func (p *inMemorySessionStorage) Store(ctx context.Context, session *domain.Session) error {
	p.mx.Lock()
	defer p.mx.Unlock()
	p.store[session.ID()] = session
	return nil
}

func (p *inMemorySessionStorage) Find(ctx context.Context, token string) (*domain.Session, error) {
	p.mx.Lock()
	defer p.mx.Unlock()
	session, ok := p.store[token]
	if !ok {
		return nil, domain.ErrSessionNotFound
	}

	return session, nil
}
