package app

import (
	"context"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
)

type CartService struct {
	cart           *domain.Cart
	productCatalog ProductCatalog
}

type CartStorage interface {
	Get(ctx context.Context, key string) (*domain.Cart, error)
}

type ProductCatalog interface {
	Find(ctx context.Context, productID string) (domain.Product, error)
}

func NewCartService(cart *domain.Cart, pc ProductCatalog) CartService {
	return CartService{cart: cart, productCatalog: pc}
}

func (c CartService) TotalPrice() (float64, error) {
	return c.cart.TotalPrice()
}

func (c CartService) Add(ctx context.Context, productID string, quantity int) error {
	p, err := c.productCatalog.Find(ctx, productID)
	if err != nil {
		return err
	}

	if err = c.cart.Add(p, quantity); err != nil {
		return fmt.Errorf("could not add product %s to the cart: %w", productID, err)
	}

	return nil
}
