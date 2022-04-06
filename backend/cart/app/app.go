package app

import (
	"context"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type CartService struct {
	storage        CartStorage
	productCatalog ProductCatalog
}

type CartStorage interface {
	Get(ctx context.Context, key string) (domain.Cart, error)
}

type ProductCatalog interface {
	Find(ctx context.Context, productID string) (domain.Product, error)
}

func NewCartService(cs CartStorage, pc ProductCatalog) CartService {
	return CartService{storage: cs, productCatalog: pc}
}

func (c CartService) Add(ctx context.Context, cartID, productID string, quantity float64) error {
	p, err := c.productCatalog.Find(ctx, productID)
	if err != nil {
		return fmt.Errorf("could not find product %s in the product catalog: %w", productID, err)
	}

	cart, err := c.storage.Get(ctx, cartID)
	if err != nil {
		return fmt.Errorf("could not get cart: %w", err)
	}

	if err = cart.Add(p, quantity); err != nil {
		return fmt.Errorf("could not add product %s to the cart: %w", productID, err)
	}

	return nil
}
