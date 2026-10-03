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

func (i *inMemory) Clear(ctx context.Context, user domain.User) error {
	i.mx.Lock()
	defer i.mx.Unlock()

	delete(i.carts, user.ID())
	return nil
}

func (i *inMemory) UpdateItemPrice(ctx context.Context, variantID string, priceMinorUnits int64, currency string) error {
	i.mx.Lock()
	defer i.mx.Unlock()

	cur, err := domain.NewCurrency(currency)
	if err != nil {
		return err
	}

	for _, cart := range i.carts {
		for _, item := range cart.Items() {
			if item.Product().ID() == variantID {
				newProduct := domain.NewProduct(variantID, item.Product().Name(), priceMinorUnits, cur)
				cart.UpdateProduct(newProduct)
			}
		}
	}
	return nil
}

func (i *inMemory) UpdateItemName(ctx context.Context, variantID string, newName string) error {
	i.mx.Lock()
	defer i.mx.Unlock()

	for _, cart := range i.carts {
		for _, item := range cart.Items() {
			if item.Product().ID() == variantID {
				newProduct := domain.NewProduct(variantID, newName, item.Product().Price().Amount(), item.Product().Price().Currency())
				cart.UpdateProduct(newProduct)
			}
		}
	}
	return nil
}
