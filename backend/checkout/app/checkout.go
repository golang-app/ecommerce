package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

// CartReader returns the current cart for a session. It deliberately mirrors
// the subset of cart.CartService that checkout needs so the dependency stays
// minimal.
type CartReader interface {
	Get(ctx context.Context, sessID string) (*cartDomain.Cart, error)
}

// EventPublisher publishes integration events for other bounded contexts.
// Checkout uses it instead of calling other contexts directly — e.g. it
// announces that an order was paid rather than reaching into the cart.
type EventPublisher interface {
	Publish(ctx context.Context, e eventbus.Event)
}

type OrderStorage interface {
	// Save persists the aggregate's pending events and projects them to the
	// read model.
	Save(ctx context.Context, order *domain.Order) error
	// Load rebuilds the order aggregate from its event history (write side).
	Load(ctx context.Context, id string) (*domain.Order, error)
}

// PaymentProcessor charges a card. The fake implementation always succeeds;
// a real one would talk to Stripe/Adyen/etc. and return an error on
// decline.
type PaymentProcessor interface {
	Charge(ctx context.Context, amount int64, currency, cardNumber string) error
}

// StockReserver atomically reserves (decrements) catalogue stock for the
// ordered variants, all-or-nothing, returning a non-nil error if any variant
// is short. Release returns reserved stock if the order can't complete.
// Implemented by the product catalogue.
type StockReserver interface {
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
}

// IDGenerator returns a fresh order ID. Injected so tests can substitute a
// deterministic generator.
type IDGenerator func() string

type CheckoutService struct {
	cart    CartReader
	storage OrderStorage
	payment PaymentProcessor
	stock   StockReserver
	events  EventPublisher
	newID   IDGenerator
	now     func() time.Time
}

func NewCheckoutService(
	cart CartReader,
	storage OrderStorage,
	payment PaymentProcessor,
	stock StockReserver,
	events EventPublisher,
	newID IDGenerator,
) CheckoutService {
	return CheckoutService{
		cart:    cart,
		storage: storage,
		payment: payment,
		stock:   stock,
		events:  events,
		newID:   newID,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// Place runs the checkout command: it snapshots the cart into a new order
// aggregate (OrderPlaced), attempts the charge, records the outcome
// (PaymentSucceeded / PaymentFailed), persists the resulting events, and
// clears the cart. The cardNumber is accepted for shape only — the fake
// processor ignores it.
//
// customerID may be empty for anonymous checkout; in that case the order is
// recorded but will not appear in any customer's order history.
func (s CheckoutService) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo domain.Address, shipMethod domain.ShippingMethod, payMethod domain.PaymentMethod) (domain.Order, error) {
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
	quantities := map[string]int{}
	for _, item := range cart.Items() {
		lines = append(lines, domain.NewLine(
			item.Product().ID(),
			item.Product().Name(),
			item.Quantity(),
			item.Product().Price().Amount(),
			string(item.Product().Price().Currency()),
		))
		quantities[item.Product().ID()] += item.Quantity()
	}

	// Reserve stock up front, atomically. If anything is short the order is
	// not placed at all — this is the guard against overselling under
	// concurrent checkouts.
	if err := s.stock.Reserve(ctx, quantities); err != nil {
		return domain.Order{}, fmt.Errorf("reserve stock: %w", err)
	}

	order, err := domain.PlaceOrder(s.newID(), sessID, customerID, shipTo, shipMethod, payMethod, lines, s.now())
	if err != nil {
		_ = s.stock.Release(ctx, quantities)
		return domain.Order{}, err
	}

	if chargeErr := s.payment.Charge(ctx, order.TotalAmount(), order.TotalCurrency(), cardNumber); chargeErr != nil {
		// Payment failed after reserving — give the stock back, record the
		// failed attempt, and report the decline.
		_ = s.stock.Release(ctx, quantities)
		order.MarkFailed(chargeErr.Error(), s.now())
		if err := s.storage.Save(ctx, order); err != nil {
			return domain.Order{}, fmt.Errorf("save order: %w", err)
		}
		return *order, fmt.Errorf("payment declined: %w", chargeErr)
	}

	order.MarkPaid(s.now())
	if err := s.storage.Save(ctx, order); err != nil {
		// Order couldn't be persisted; return the reservation.
		_ = s.stock.Release(ctx, quantities)
		return domain.Order{}, fmt.Errorf("save order: %w", err)
	}

	// Announce the paid order. Subscribers (e.g. the cart context emptying the
	// basket) react best-effort; their failures never block the confirmation.
	s.events.Publish(ctx, integration.OrderPaid{
		OrderID:    order.ID(),
		SessionID:  sessID,
		CustomerID: customerID,
		At:         s.now(),
	})

	return *order, nil
}

// Cancel cancels a customer's paid order: it rehydrates the aggregate from its
// events, applies the Cancel command, persists the resulting OrderCancelled
// event, and returns the reserved stock to the catalogue.
//
// Only the order's owner may cancel it; for any other customer the order is
// reported as not found so its existence isn't leaked.
func (s CheckoutService) Cancel(ctx context.Context, orderID, customerID string) error {
	order, err := s.storage.Load(ctx, orderID)
	if err != nil {
		return err
	}
	if customerID == "" || order.CustomerID() != customerID {
		return domain.ErrOrderNotFound
	}

	return s.cancel(ctx, order, "cancelled by customer")
}

// AdminCancel cancels any paid order on behalf of an administrator, without an
// ownership check. It rehydrates the aggregate, applies Cancel (returning
// ErrOrderNotCancellable for non-paid orders), persists the OrderCancelled
// event, and returns the reserved stock to the catalogue.
func (s CheckoutService) AdminCancel(ctx context.Context, orderID string) error {
	order, err := s.storage.Load(ctx, orderID)
	if err != nil {
		return err
	}
	return s.cancel(ctx, order, "cancelled by admin")
}

// cancel applies the Cancel command, persists it, and releases reserved stock
// (best-effort). Shared by the customer and admin cancellation flows.
func (s CheckoutService) cancel(ctx context.Context, order *domain.Order, reason string) error {
	if err := order.Cancel(reason, s.now()); err != nil {
		return err
	}
	if err := s.storage.Save(ctx, order); err != nil {
		return fmt.Errorf("save cancellation: %w", err)
	}

	// Return the reserved stock to the catalogue (best-effort).
	quantities := map[string]int{}
	for _, ln := range order.Items() {
		quantities[ln.ProductID()] += ln.Quantity()
	}
	_ = s.stock.Release(ctx, quantities)

	return nil
}
