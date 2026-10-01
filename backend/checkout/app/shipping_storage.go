package app

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

var ErrShippingMethodNotFound = errors.New("shipping method not found")

type ShippingStorage interface {
	ListShippingMethods(ctx context.Context) ([]domain.ShippingMethod, error)
	FindShippingMethod(ctx context.Context, code string) (domain.ShippingMethod, error)
	SaveShippingMethod(ctx context.Context, method domain.ShippingMethod) error
}

type inMemoryShippingStorage struct {
	mu      sync.RWMutex
	methods map[string]domain.ShippingMethod
}

func NewInMemoryShippingStorage() ShippingStorage {
	storage := &inMemoryShippingStorage{
		methods: make(map[string]domain.ShippingMethod),
	}

	storage.methods["flat"] = domain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true)
	storage.methods["pickup"] = domain.NewShippingMethod("pickup", "Personal pickup", 0, false, "Store Pickup", true)
	storage.methods["courier"] = domain.NewShippingMethod("courier", "Courier", 1500, true, "Express Courier", true)

	return storage
}

func (s *inMemoryShippingStorage) ListShippingMethods(ctx context.Context) ([]domain.ShippingMethod, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.ShippingMethod, 0, len(s.methods))
	for _, m := range s.methods {
		out = append(out, m)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Code() < out[j].Code()
	})

	return out, nil
}

func (s *inMemoryShippingStorage) FindShippingMethod(ctx context.Context, code string) (domain.ShippingMethod, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.methods[code]
	if !ok {
		return domain.ShippingMethod{}, ErrShippingMethodNotFound
	}

	return m, nil
}

func (s *inMemoryShippingStorage) SaveShippingMethod(ctx context.Context, method domain.ShippingMethod) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.methods[method.Code()] = method
	return nil
}

