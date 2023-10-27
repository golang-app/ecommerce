package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type CartService struct {
	storage        CartStorage
	productCatalog ProductCatalog
}

type CartStorage interface {
	Get(ctx context.Context, user domain.User) (*domain.Cart, error)
	Persist(ctx context.Context, cart *domain.Cart) error
}

type ProductCatalog interface {
	Find(ctx context.Context, productID string) (domain.Product, error)
}

func NewCartService(storage CartStorage, pc ProductCatalog) CartService {
	return CartService{storage: storage, productCatalog: pc}
}

func (c CartService) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	user := domain.NewUser(sessID)
	p, err := c.productCatalog.Find(ctx, productID)
	if err != nil {
		return fmt.Errorf("could not find product (%s): %w", productID, err)
	}

	cart, err := c.storage.Get(ctx, user)
	if errors.Is(err, domain.ErrCartNotFound) {
		err = nil
		cart = domain.NewCart(user)
	}

	if err != nil {
		return fmt.Errorf("could not get cart: %w", err)
	}

	err = cart.Add(p, qty)
	if err != nil {
		return fmt.Errorf("could not add product to cart: %w", err)
	}

	err = c.storage.Persist(ctx, cart)
	if err != nil {
		return fmt.Errorf("could not persist cart: %w", err)
	}

	return nil
}

func (c CartService) Get(ctx context.Context, sessID string) (*domain.Cart, error) {
	user := domain.NewUser(sessID)

	cart, err := c.storage.Get(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("could not get cart: %w", err)
	}

	return cart, nil
}
