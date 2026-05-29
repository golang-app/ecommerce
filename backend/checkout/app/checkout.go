package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer is the package-level tracer for the checkout application service.
// The parent Place span and its named child spans (cart.get, stock.reserve,
// payment.charge, order.save) all flow through this scope so they share a
// single instrumentation name in the trace backend. The previous
// events.publish span has been retired with the inline event bus call —
// integration events are now staged into the outbox inside order.save and
// published asynchronously by the outbox dispatcher.
var tracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/checkout/app")

// recordSpanError marks the span errored and attaches the underlying error so
// trace consumers can spot failed operations without poking through
// attributes. Centralising the two-call idiom keeps the spans uniform.
func recordSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// totalReservedUnits sums the quantity values in a reservation/release map.
// Used to drive the gocommerce_stock_reserved_total / _released_total
// counters in variant-units regardless of how many distinct variants the
// order spans.
func totalReservedUnits(quantities map[string]int) int64 {
	var total int64
	for _, q := range quantities {
		total += int64(q)
	}
	return total
}

// CartReader returns the current cart for a session. It deliberately mirrors
// the subset of cart.CartService that checkout needs so the dependency stays
// minimal.
type CartReader interface {
	Get(ctx context.Context, sessID string) (*cartDomain.Cart, error)
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

// StockMovements is the audit-log seam: every reservation/release/refund the
// checkout context drives also records a row in the catalogue's stock
// movement log. It is intentionally separate from StockReserver so we don't
// have to widen the existing Reserve/Release signatures (which other
// contexts call). A no-op implementation (nopStockMovements) is used when no
// recorder is wired so existing tests stay unchanged.
type StockMovements interface {
	Record(ctx context.Context, variantID string, delta int, reason, refOrderID string) error
}

// nopStockMovements is the default StockMovements when none is supplied. It
// keeps the audit trail strictly additive: contexts that don't care simply
// see no log rows.
type nopStockMovements struct{}

func (nopStockMovements) Record(context.Context, string, int, string, string) error { return nil }

// PricingPolicy is an alias for the domain-level policy so the
// composition root (cmd/web) can continue to construct one through the
// checkout/app package while the actual math lives in the Pricing
// domain service (checkout/domain.PriceQuote). The field set —
// TaxRatePercent + FreeShippingThreshold — is unchanged, which keeps
// the historical wiring arithmetic-identical to before the lift.
type PricingPolicy = domain.PricingPolicy

// IDGenerator returns a fresh order ID. Injected so tests can substitute a
// deterministic generator.
type IDGenerator func() string

// PromoRedeemer is the seam to the promo bounded context. Place calls
// Redeem AFTER the order's events are committed so the redemption ledger
// only grows for orders that actually exist. A nil PromoRedeemer disables
// the integration; the in-memory wiring used by tests doesn't need it.
type PromoRedeemer interface {
	Redeem(ctx context.Context, code, orderID, customerID string, discount promodomain.Discount) error
}

// nopPromoRedeemer is the default when no promo integration is wired —
// keeps the service constructible without a promo dependency.
type nopPromoRedeemer struct{}

func (nopPromoRedeemer) Redeem(context.Context, string, string, string, promodomain.Discount) error {
	return nil
}

type CheckoutService struct {
	cart      CartReader
	storage   OrderStorage
	payment   PaymentProcessor
	stock     StockReserver
	movements StockMovements
	promo     PromoRedeemer
	newID     IDGenerator
	now       func() time.Time
	pricing   PricingPolicy
}

// NewCheckoutService constructs the checkout command service.
//
// Note: there is no EventPublisher dependency. Integration events
// (e.g. OrderPaid) are staged into the Transactional Outbox by the
// storage layer inside Save's own transaction, then published by a
// separate dispatcher. The application service is intentionally
// oblivious to that wiring — it only knows how to call Save.
func NewCheckoutService(
	cart CartReader,
	storage OrderStorage,
	payment PaymentProcessor,
	stock StockReserver,
	movements StockMovements,
	newID IDGenerator,
	pricing PricingPolicy,
) CheckoutService {
	if movements == nil {
		movements = nopStockMovements{}
	}
	return CheckoutService{
		cart:      cart,
		storage:   storage,
		payment:   payment,
		stock:     stock,
		movements: movements,
		promo:     nopPromoRedeemer{},
		newID:     newID,
		now:       func() time.Time { return time.Now().UTC() },
		pricing:   pricing,
	}
}

// WithPromoRedeemer wires the promo redemption seam — only the
// composition root in cmd/web calls this so the historical zero-promo
// wiring stays untouched in tests.
func (s CheckoutService) WithPromoRedeemer(p PromoRedeemer) CheckoutService {
	if p == nil {
		p = nopPromoRedeemer{}
	}
	s.promo = p
	return s
}

// Place runs the checkout command: it snapshots the cart into a new order
// aggregate (OrderPlaced), attempts the charge, records the outcome
// (PaymentSucceeded / PaymentFailed), persists the resulting events, and
// clears the cart. The cardNumber is accepted for shape only — the fake
// processor ignores it.
//
// customerID may be empty for anonymous checkout; in that case the order is
// recorded but will not appear in any customer's order history.
//
// discount carries an already-resolved promo code (see promo/app.Service.
// Resolve). The pricing math — discount-before-tax, free-shipping
// override, total — lives in the domain.PriceQuote service; this
// orchestrator just hands it the inputs and reads the quote back.
//
// An empty Discount value (no code applied) is the same arithmetic as
// before promo codes existed.
func (s CheckoutService) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo domain.Address, shipMethod domain.ShippingMethod, payMethod domain.PaymentMethod, discount promodomain.Discount) (domain.Order, error) {
	ctx, span := tracer.Start(ctx, "Checkout.Place", trace.WithAttributes(
		attribute.String("cart.session_id", sessID),
		attribute.String("customer.id", customerID),
		attribute.String("payment.method", payMethod.Code()),
		attribute.String("ship.method", shipMethod.Code()),
	))
	defer span.End()

	// cart.get is a leaf child span so the trace clearly shows whether the
	// hand-off into the cart context is the slow step.
	cartCtx, cartSpan := tracer.Start(ctx, "cart.get", trace.WithAttributes(
		attribute.String("cart.session_id", sessID),
	))
	cart, err := s.cart.Get(cartCtx, sessID)
	if err != nil {
		recordSpanError(cartSpan, err)
		cartSpan.End()
		if errors.Is(err, cartDomain.ErrCartNotFound) {
			recordSpanError(span, domain.ErrCartEmpty)
			return domain.Order{}, domain.ErrCartEmpty
		}
		recordSpanError(span, err)
		return domain.Order{}, fmt.Errorf("get cart: %w", err)
	}
	cartSpan.SetAttributes(attribute.Int("cart.item_count", len(cart.Items())))
	cartSpan.End()
	if len(cart.Items()) == 0 {
		recordSpanError(span, domain.ErrCartEmpty)
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

	reservedUnits := totalReservedUnits(quantities)

	// Reserve stock up front, atomically. If anything is short the order is
	// not placed at all — this is the guard against overselling under
	// concurrent checkouts.
	reserveCtx, reserveSpan := tracer.Start(ctx, "stock.reserve", trace.WithAttributes(
		attribute.Int("stock.variants", len(quantities)),
		attribute.Int64("stock.total_units", reservedUnits),
	))
	if err := s.stock.Reserve(reserveCtx, quantities); err != nil {
		recordSpanError(reserveSpan, err)
		reserveSpan.End()
		recordSpanError(span, err)
		return domain.Order{}, fmt.Errorf("reserve stock: %w", err)
	}
	reserveSpan.End()
	observability.StockReservedAdd(ctx, reservedUnits)

	orderID := s.newID()
	span.SetAttributes(attribute.String("order.id", orderID))

	// Record reservation movements (best-effort: a failure here must not
	// undo the reservation itself).
	for vid, qty := range quantities {
		_ = s.movements.Record(ctx, vid, -qty, "reserve", orderID)
	}

	// Pricing math is delegated to the Pricing domain service so this
	// orchestrator stays focused on the workflow. The aggregate's
	// constructor still takes the resolved tax / shipping / discount
	// values — we just no longer compute them here.
	quote := domain.PriceQuote(lines, shipMethod, domain.DiscountInput{
		AmountMinor:  discount.AmountMinor(),
		FreeShipping: discount.FreeShipping(),
	}, s.pricing)
	span.SetAttributes(
		attribute.Int64("order.subtotal", quote.Subtotal),
		attribute.Int64("order.discount", quote.DiscountAmount),
		attribute.String("order.discount_code", discount.Code()),
		attribute.Int64("order.tax", quote.Tax),
		attribute.Int64("order.shipping", quote.ShippingCost),
	)

	// channel records the sales channel the order was placed through; the
	// admin/checkout HTTP flow is the only producer today, so we hardcode
	// "web". Pluggable when iOS / API channels arrive — the signature is
	// already wired through PlaceOrder and the OrderPlaced v2 schema.
	const channel = "web"
	order, err := domain.PlaceOrder(orderID, sessID, customerID, shipTo, shipMethod, payMethod, lines, quote.Tax, quote.ShippingCost, discount.Code(), quote.DiscountAmount, channel, s.now())
	if err != nil {
		_ = s.stock.Release(ctx, quantities)
		for vid, qty := range quantities {
			_ = s.movements.Record(ctx, vid, qty, "release-place-failed", orderID)
		}
		observability.StockReleasedAdd(ctx, reservedUnits)
		recordSpanError(span, err)
		return domain.Order{}, err
	}
	span.SetAttributes(
		attribute.Int64("order.total", order.TotalAmount()),
		attribute.String("order.currency", order.TotalCurrency()),
	)

	// One placed-order counter increment per successful aggregate (before
	// payment outcome): keeps Grafana's checkout-funnel view honest by
	// separating "we attempted" from "we charged".
	observability.OrdersPlacedInc(ctx, payMethod.Code(), shipMethod.Code())

	chargeCtx, chargeSpan := tracer.Start(ctx, "payment.charge", trace.WithAttributes(
		attribute.Int64("payment.amount", order.TotalAmount()),
		attribute.String("payment.currency", order.TotalCurrency()),
		attribute.String("payment.method", payMethod.Code()),
	))
	chargeErr := s.payment.Charge(chargeCtx, order.TotalAmount(), order.TotalCurrency(), cardNumber)
	if chargeErr != nil {
		recordSpanError(chargeSpan, chargeErr)
		chargeSpan.End()
		// Payment failed after reserving — give the stock back, record the
		// failed attempt, and report the decline.
		_ = s.stock.Release(ctx, quantities)
		for vid, qty := range quantities {
			_ = s.movements.Record(ctx, vid, qty, "release-failed-payment", orderID)
		}
		observability.PaymentsFailedInc(ctx)
		observability.StockReleasedAdd(ctx, reservedUnits)
		observability.OrdersFinalizedInc(ctx, string(domain.StatusFailed))
		order.MarkFailed(chargeErr.Error(), s.now())
		saveCtx, saveSpan := tracer.Start(ctx, "order.save", trace.WithAttributes(
			attribute.String("order.id", orderID),
			attribute.String("order.status", string(domain.StatusFailed)),
		))
		if err := s.storage.Save(saveCtx, order); err != nil {
			recordSpanError(saveSpan, err)
			saveSpan.End()
			recordSpanError(span, err)
			return domain.Order{}, fmt.Errorf("save order: %w", err)
		}
		saveSpan.End()
		err := fmt.Errorf("payment declined: %w", chargeErr)
		recordSpanError(span, err)
		return *order, err
	}
	chargeSpan.End()
	observability.PaymentsChargedInc(ctx, order.TotalCurrency())

	order.MarkPaid(s.now())
	saveCtx, saveSpan := tracer.Start(ctx, "order.save", trace.WithAttributes(
		attribute.String("order.id", orderID),
		attribute.String("order.status", string(domain.StatusPaid)),
	))
	if err := s.storage.Save(saveCtx, order); err != nil {
		recordSpanError(saveSpan, err)
		saveSpan.End()
		// Order couldn't be persisted; return the reservation.
		_ = s.stock.Release(ctx, quantities)
		for vid, qty := range quantities {
			_ = s.movements.Record(ctx, vid, qty, "release-save-failed", orderID)
		}
		observability.StockReleasedAdd(ctx, reservedUnits)
		recordSpanError(span, err)
		return domain.Order{}, fmt.Errorf("save order: %w", err)
	}
	saveSpan.End()

	// Revenue + paid-finalisation are recorded only after the save succeeds
	// so a write failure doesn't double-book the totals.
	observability.RevenueAdd(ctx, order.TotalAmount(), order.TotalCurrency())
	observability.OrdersFinalizedInc(ctx, string(domain.StatusPaid))

	// Record the promo redemption (best-effort, like the cart-clear): the
	// order is now committed so a failed redemption ledger insertion must
	// not roll the order back. The (code, order_id) PK on
	// promo_redemption keeps the call idempotent if a retry lands.
	if discount.Code() != "" {
		if err := s.promo.Redeem(ctx, discount.Code(), order.ID(), customerID, discount); err != nil {
			span.AddEvent("promo.redeem.failed", trace.WithAttributes(
				attribute.String("promo.code", discount.Code()),
				attribute.String("error", err.Error()),
			))
		}
	}

	// Integration events are no longer announced inline. The Save above
	// has already staged an OrderPaid row into the outbox inside the
	// same transaction that committed the domain events — the
	// background dispatcher will publish it to the bus. That's what
	// makes the hand-off durable across crashes between commit and
	// publish.

	return *order, nil
}

// Cancel cancels a customer's paid order: it rehydrates the aggregate from its
// events, applies the Cancel command, persists the resulting OrderCancelled
// event, and returns the reserved stock to the catalogue.
//
// Only the order's owner may cancel it; for any other customer the order is
// reported as not found so its existence isn't leaked.
func (s CheckoutService) Cancel(ctx context.Context, orderID, customerID string) error {
	ctx, span := tracer.Start(ctx, "Checkout.Cancel", trace.WithAttributes(
		attribute.String("order.id", orderID),
		attribute.String("customer.id", customerID),
	))
	defer span.End()

	order, err := s.storage.Load(ctx, orderID)
	if err != nil {
		recordSpanError(span, err)
		return err
	}
	if customerID == "" || order.CustomerID() != customerID {
		recordSpanError(span, domain.ErrOrderNotFound)
		return domain.ErrOrderNotFound
	}

	if err := s.cancel(ctx, order, "cancelled by customer"); err != nil {
		recordSpanError(span, err)
		return err
	}
	return nil
}

// AdminCancel cancels any paid order on behalf of an administrator, without an
// ownership check. It rehydrates the aggregate, applies Cancel (returning
// ErrOrderNotCancellable for non-paid orders), persists the OrderCancelled
// event, and returns the reserved stock to the catalogue.
func (s CheckoutService) AdminCancel(ctx context.Context, orderID string) error {
	ctx, span := tracer.Start(ctx, "Checkout.AdminCancel", trace.WithAttributes(
		attribute.String("order.id", orderID),
	))
	defer span.End()

	order, err := s.storage.Load(ctx, orderID)
	if err != nil {
		recordSpanError(span, err)
		return err
	}
	if err := s.cancel(ctx, order, "cancelled by admin"); err != nil {
		recordSpanError(span, err)
		return err
	}
	return nil
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

	s.releaseStock(ctx, order, "release-cancel")
	// State transition counter: status=cancelled regardless of whether the
	// trigger was a customer or admin command.
	observability.OrdersFinalizedInc(ctx, string(domain.StatusCancelled))
	return nil
}

// releaseStock returns every line's quantity to the catalogue (best-effort)
// and records a movement per variant for the audit trail. reason describes
// why the release happened (e.g. "release", "release-refund").
func (s CheckoutService) releaseStock(ctx context.Context, order *domain.Order, reason string) {
	quantities := map[string]int{}
	for _, ln := range order.Items() {
		quantities[ln.ProductID()] += ln.Quantity()
	}
	if len(quantities) == 0 {
		return
	}
	_ = s.stock.Release(ctx, quantities)
	for vid, qty := range quantities {
		_ = s.movements.Record(ctx, vid, qty, reason, order.ID())
	}
	observability.StockReleasedAdd(ctx, totalReservedUnits(quantities))
}

// ExpirePending fails a pending order whose reservation TTL has elapsed: it
// rehydrates the aggregate, marks it failed with a "reservation expired"
// reason, persists the event, and releases the reserved stock. It is
// idempotent — if the order is no longer pending (another worker raced us, or
// the customer's payment finally landed) it returns nil without touching the
// aggregate. Used by the reservation TTL sweeper.
func (s CheckoutService) ExpirePending(ctx context.Context, orderID string) error {
	ctx, span := tracer.Start(ctx, "Checkout.ExpirePending", trace.WithAttributes(
		attribute.String("order.id", orderID),
	))
	defer span.End()

	order, err := s.storage.Load(ctx, orderID)
	if err != nil {
		recordSpanError(span, err)
		return err
	}
	if order.Status() != domain.StatusPending {
		// Not an error path — another worker raced us or the customer
		// just paid. Annotate the span so trace consumers see the
		// no-op cleanly.
		span.SetAttributes(attribute.String("expire.outcome", "noop"))
		return nil
	}
	order.MarkFailed("reservation expired", s.now())
	if err := s.storage.Save(ctx, order); err != nil {
		recordSpanError(span, err)
		return fmt.Errorf("save expired order: %w", err)
	}
	s.releaseStock(ctx, order, "release-expired")
	// State transition counter: an expired pending is a failed terminal
	// state from the funnel's perspective.
	observability.OrdersFinalizedInc(ctx, string(domain.StatusFailed))
	return nil
}

// MarkShipped / MarkDelivered / Refund used to live on this service:
// they applied the matching domain commands on the Order aggregate
// and (for Refund) released stock back to the catalogue. That
// behaviour now lives in the fulfillment bounded context's Process
// Manager (backend/fulfillment), where the operational lifecycle —
// scheduled → labeled → shipped → delivered / refunded — is modelled
// as its own state machine. See the package doc on
// backend/fulfillment/domain for the rationale (commercial vs
// operational state separation).
//
// The Order aggregate's MarkShipped / MarkDelivered / Refund methods
// stay on checkout/domain for replay / back-compat: historical event
// logs that include OrderShipped / OrderDelivered / OrderRefunded
// still apply cleanly when rebuilding read models or aggregates.
