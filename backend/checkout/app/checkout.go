package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

// CartReader returns the current cart for a session. It deliberately mirrors
// the subset of cart.CartService that checkout needs so the dependency stays
// minimal.
type CartReader interface {
	Get(ctx context.Context, sessID string) (*cartDomain.Cart, error)
}

// CartClearer empties the cart for a session after a successful order.
type CartClearer interface {
	Clear(ctx context.Context, sessID string) error
}

type OrderStorage interface {
	Save(ctx context.Context, order domain.Order) error
	Find(ctx context.Context, id string) (domain.Order, error)
	ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error)
}

// PaymentProcessor charges a card. The fake implementation always succeeds;
// a real one would talk to Stripe/Adyen/etc. and return an error on
// decline.
type PaymentProcessor interface {
	Charge(ctx context.Context, amount int64, currency, cardNumber string) error
}

// IDGenerator returns a fresh order ID. Injected so tests can substitute a
// deterministic generator.
type IDGenerator func() string

type CheckoutService struct {
	cart    CartReader
	cartClr CartClearer
	storage OrderStorage
	payment PaymentProcessor
	newID   IDGenerator
	now     func() time.Time
}

func NewCheckoutService(
	cart CartReader,
	cartClr CartClearer,
	storage OrderStorage,
	payment PaymentProcessor,
	newID IDGenerator,
) CheckoutService {
	return CheckoutService{
		cart:    cart,
		cartClr: cartClr,
		storage: storage,
		payment: payment,
		newID:   newID,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Place creates an order from the session's current cart, charges the
// supplied fake card, persists the order, and clears the cart. The
// cardNumber argument is accepted for shape only — the fake processor
// ignores it.
//
// customerID may be empty for anonymous checkout; in that case the order
// is recorded but will not appear in any customer's order history.
func (s CheckoutService) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo domain.Address, shipMethod domain.ShippingMethod) (domain.Order, error) {
	cart, err := s.cart.Get(ctx, sessID)
	if err != nil {
		if errors.Is(err, cartDomain.ErrCartNotFound) {
			return domain.Order{}, domain.ErrCartEmpty
		}
		return domain.Order{}, fmt.Errorf("get cart: %w", err)
	}
	if len(cart.Items()) == 0 {
		return domain.Order{}, domain.ErrCartEmpty
	}

	lines := make([]domain.Line, 0, len(cart.Items()))
	for _, item := range cart.Items() {
		lines = append(lines, domain.NewLine(
			item.Product().ID(),
			item.Product().Name(),
			item.Quantity(),
			item.Product().Price().Amount(),
			string(item.Product().Price().Currency()),
		))
	}

	order := domain.NewOrder(s.newID(), sessID, customerID, shipTo, shipMethod, lines, domain.StatusPending, s.now())

	if err := s.payment.Charge(ctx, order.TotalAmount(), order.TotalCurrency(), cardNumber); err != nil {
		failed := order.WithStatus(domain.StatusFailed)
		_ = s.storage.Save(ctx, failed)
		return failed, fmt.Errorf("payment declined: %w", err)
	}

	paid := order.WithStatus(domain.StatusPaid)
	if err := s.storage.Save(ctx, paid); err != nil {
		return domain.Order{}, fmt.Errorf("save order: %w", err)
	}

	// Cart clear failure is non-fatal — the order is already placed and the
	// customer should see the confirmation page.
	_ = s.cartClr.Clear(ctx, sessID)

	return paid, nil
}

func (s CheckoutService) Find(ctx context.Context, id string) (domain.Order, error) {
	return s.storage.Find(ctx, id)
}

// ListByCustomer returns the authenticated customer's orders newest-first.
// Anonymous orders (placed with an empty customerID) are never returned
// here regardless of the value passed; this is enforced by Postgres treating
// NULL = '' as false.
func (s CheckoutService) ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error) {
	if customerID == "" {
		return nil, nil
	}
	return s.storage.ListByCustomer(ctx, customerID)
}
