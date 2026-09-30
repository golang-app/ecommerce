package app

import (
	"context"
	"errors"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

var ErrShippingMethodNotFound = errors.New("shipping method not found")

type ShippingStorage interface {
	ListShippingMethods(ctx context.Context) ([]domain.ShippingMethod, error)
	FindShippingMethod(ctx context.Context, code string) (domain.ShippingMethod, error)
	SaveShippingMethod(ctx context.Context, method domain.ShippingMethod) error
}
