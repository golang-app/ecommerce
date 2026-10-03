package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the package-level tracer used to start spans from the cart
// application layer. It is bound to the OTel global TracerProvider — when no
// exporter is configured the returned tracer is a no-op so Start/End calls
// add no measurable overhead.
var tracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/cart/app")

type CartService struct {
	storage        CartStorage
	productCatalog ProductCatalog
}

type CartStorage interface {
	Get(ctx context.Context, user domain.User) (*domain.Cart, error)
	Persist(ctx context.Context, cart *domain.Cart) error
	Clear(ctx context.Context, user domain.User) error
	UpdateItemPrice(ctx context.Context, variantID string, priceMinorUnits int64, currency string) error
	UpdateItemName(ctx context.Context, variantID string, newName string) error
}

type ProductCatalog interface {
	Find(ctx context.Context, variantID string) (domain.Product, error)
}

func NewCartService(storage CartStorage, pc ProductCatalog) CartService {
	return CartService{storage: storage, productCatalog: pc}
}

func (c CartService) AddToCart(ctx context.Context, sessID string, variantID string, qty int) error {
	ctx, span := tracer.Start(ctx, "Cart.AddToCart", trace.WithAttributes(
		attribute.String("cart.session_id", sessID),
		attribute.String("variant.id", variantID),
		attribute.Int("cart.qty", qty),
	))
	defer span.End()

	user := domain.NewUser(sessID)
	p, err := c.productCatalog.Find(ctx, variantID)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not find product for variant (%s): %w", variantID, err)
	}

	cart, err := c.storage.Get(ctx, user)
	if errors.Is(err, domain.ErrCartNotFound) {
		err = nil
		cart = domain.NewCart(user)
	}

	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not get cart: %w", err)
	}

	err = cart.Add(p, qty)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not add product to cart: %w", err)
	}

	err = c.storage.Persist(ctx, cart)
	if err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not persist cart: %w", err)
	}

	observability.CartItemsAddedInc(ctx)
	return nil
}

func (c CartService) Get(ctx context.Context, sessID string) (*domain.Cart, error) {
	ctx, span := tracer.Start(ctx, "Cart.Get", trace.WithAttributes(
		attribute.String("cart.session_id", sessID),
	))
	defer span.End()

	user := domain.NewUser(sessID)

	cart, err := c.storage.Get(ctx, user)
	if err != nil {
		recordSpanError(span, err)
		return nil, fmt.Errorf("could not get cart: %w", err)
	}

	return cart, nil
}

// Clear removes every item from the cart for this session. The cart row
// itself is left in place; the next Get returns an empty cart.
func (c CartService) Clear(ctx context.Context, sessID string) error {
	ctx, span := tracer.Start(ctx, "Cart.Clear", trace.WithAttributes(
		attribute.String("cart.session_id", sessID),
	))
	defer span.End()

	user := domain.NewUser(sessID)
	if err := c.storage.Clear(ctx, user); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not clear cart: %w", err)
	}
	return nil
}

// UpdateItemPrice synchronizes existing cart items matching variantID to the new price and currency.
func (c CartService) UpdateItemPrice(ctx context.Context, variantID string, priceMinorUnits int64, currency string) error {
	ctx, span := tracer.Start(ctx, "Cart.UpdateItemPrice", trace.WithAttributes(
		attribute.String("variant.id", variantID),
		attribute.Int64("price.amount", priceMinorUnits),
		attribute.String("price.currency", currency),
	))
	defer span.End()

	if err := c.storage.UpdateItemPrice(ctx, variantID, priceMinorUnits, currency); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not update item price: %w", err)
	}
	return nil
}

// UpdateItemName synchronizes existing cart items matching variantID to the new product name.
func (c CartService) UpdateItemName(ctx context.Context, variantID string, newName string) error {
	ctx, span := tracer.Start(ctx, "Cart.UpdateItemName", trace.WithAttributes(
		attribute.String("variant.id", variantID),
		attribute.String("product.new_name", newName),
	))
	defer span.End()

	if err := c.storage.UpdateItemName(ctx, variantID, newName); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("could not update item name: %w", err)
	}
	return nil
}

// recordSpanError flags the active span as errored and records the underlying
// error. Keeping the two-step idiom in a single helper makes the call sites
// uniform and avoids drift between status and recorded events.
func recordSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
