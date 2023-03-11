package adapter

import (
	"context"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type inMemory struct {
	mx    sync.Mutex
	carts map[string]*domain.Cart
}

func NewInMemory() *inMemory {
	return &inMemory{
		carts: make(map[string]*domain.Cart),
	}
}

func (i *inMemory) Get(ctx context.Context, user domain.User) (*domain.Cart, error) {
	i.mx.Lock()
	defer i.mx.Unlock()

	cart, ok := i.carts[user.ID()]
	if !ok {
		return nil, domain.ErrCartNotFound
	}

	return cart, nil
}

func (i *inMemory) Persist(ctx context.Context, cart *domain.Cart) error {
	i.mx.Lock()
	defer i.mx.Unlock()

	i.carts[cart.User().ID()] = cart

	return nil
}
