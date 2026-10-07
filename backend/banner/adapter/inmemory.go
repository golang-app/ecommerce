package adapter

import (
	"context"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/banner/domain"
)

type InMemory struct {
	mu      sync.RWMutex
	banners map[string]domain.Banner
}

func NewInMemory() *InMemory {
	return &InMemory{
		banners: make(map[string]domain.Banner),
	}
}

func (m *InMemory) Get(_ context.Context, id string) (domain.Banner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.banners[id]
	if !ok {
		return domain.Banner{}, domain.ErrBannerNotFound
	}
	return b, nil
}

func (m *InMemory) ListAll(_ context.Context) ([]domain.Banner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Banner, 0, len(m.banners))
	for _, b := range m.banners {
		out = append(out, b)
	}
	return out, nil
}

func (m *InMemory) ListActiveBySlot(_ context.Context, slot domain.Slot) ([]domain.Banner, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []domain.Banner
	for _, b := range m.banners {
		if b.Slot == slot && b.IsActive {
			out = append(out, b)
		}
	}
	return out, nil
}

func (m *InMemory) Store(_ context.Context, b domain.Banner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.banners[b.ID] = b
	return nil
}

func (m *InMemory) Update(_ context.Context, b domain.Banner) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.banners[b.ID]; !ok {
		return domain.ErrBannerNotFound
	}
	m.banners[b.ID] = b
	return nil
}

func (m *InMemory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.banners[id]; !ok {
		return domain.ErrBannerNotFound
	}
	delete(m.banners, id)
	return nil
}
