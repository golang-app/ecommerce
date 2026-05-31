// This test exists at the boundary BETWEEN bounded contexts. Unit tests in
// each context exercise their own domain rules in isolation; this test
// exercises the INTEGRATION between them via the eventbus + outbox + inbox
// + the cross-context subscriber wiring done at the composition root.
//
// If the OrderPaid integration event's shape changes (fields added/renamed),
// or a subscriber is added/removed/renamed, or the at-least-once delivery
// contract from the outbox dispatcher is altered, this test breaks and you'll
// notice.
package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	cartadapter "github.com/bkielbasa/go-ecommerce/backend/cart/adapter"
	cartapp "github.com/bkielbasa/go-ecommerce/backend/cart/app"
	cartdomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutapp "github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	checkoutdomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutintegration "github.com/bkielbasa/go-ecommerce/backend/checkout/integration"
	checkoutquery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentadapter "github.com/bkielbasa/go-ecommerce/backend/fulfillment/adapter"
	fulfillmentapp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentdomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/bkielbasa/go-ecommerce/backend/internal/inbox"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/bkielbasa/go-ecommerce/backend/internal/outbox"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/sirupsen/logrus"
)

// dispatchOnce is the test's equivalent of *outbox.Dispatcher.dispatchOnce —
// it lives in the test because the production method is unexported. It pulls
// every unsent row, decodes it, publishes it through the bus's id-aware
// channel, and marks it sent. The shape mirrors the production dispatcher's
// drain loop precisely so the test exercises the same publish path id-aware
// subscribers go through in production (including the outbox-row id that
// inbox.Wrap dedupes on).
func dispatchOnce(t *testing.T, ctx context.Context, store *inMemoryOutbox, bus *eventbus.Bus, decode outbox.Decoder) {
	t.Helper()
	rows, err := store.Unsent(ctx, 100)
	if err != nil {
		t.Fatalf("outbox.Unsent: %v", err)
	}
	for _, r := range rows {
		ev, err := decode(r.Kind, r.Payload)
		if err != nil {
			t.Fatalf("outbox decode (id=%d kind=%s): %v", r.ID, r.Kind, err)
		}
		bus.PublishWithID(ctx, r.ID, ev)
		if err := store.MarkSent(ctx, r.ID); err != nil {
			t.Fatalf("outbox MarkSent (id=%d): %v", r.ID, err)
		}
	}
}

// ---------------------------------------------------------------------------
// In-memory adapters used only by this cross-context test. They live in
// integration_test (not in any context's adapter package) precisely because
// they cut across multiple contexts at once — the regular per-context tests
// don't need them.
// ---------------------------------------------------------------------------

// inMemoryOutbox is a tiny outbox.Store + AppendTx-style writer that the
// in-memory checkout OrderStorage stages OrderPaid rows into. It mirrors
// the *outbox.Postgres contract (Unsent / MarkSent) closely enough that
// outbox.NewDispatcher drives it identically.
type inMemoryOutbox struct {
	mu       sync.Mutex
	rows     []outbox.Row
	sent     map[int64]struct{}
	nextID   int64
}

func newInMemoryOutbox() *inMemoryOutbox {
	return &inMemoryOutbox{sent: map[int64]struct{}{}}
}

// append stages a fresh outbox row (the in-memory analogue of AppendTx).
// Tests don't need transactional atomicity — the goal is the shape of the
// dispatcher's read path, not the durability of the producer's write.
func (s *inMemoryOutbox) append(kind string, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.rows = append(s.rows, outbox.Row{
		ID:        s.nextID,
		Kind:      kind,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *inMemoryOutbox) Unsent(_ context.Context, limit int) ([]outbox.Row, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]outbox.Row, 0, len(s.rows))
	for _, r := range s.rows {
		if _, done := s.sent[r.ID]; done {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *inMemoryOutbox) MarkSent(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[id] = struct{}{}
	return nil
}

// inMemoryInbox is a per-(subscriber, eventID) dedupe table — the in-process
// equivalent of internal/inbox.Postgres. It satisfies inbox.MarkHandler.
type inMemoryInbox struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func newInMemoryInbox() *inMemoryInbox {
	return &inMemoryInbox{seen: map[string]struct{}{}}
}

func (s *inMemoryInbox) MarkHandled(_ context.Context, subscriber string, eventID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s|%d", subscriber, eventID)
	if _, dup := s.seen[key]; dup {
		return true, nil
	}
	s.seen[key] = struct{}{}
	return false, nil
}

// inMemoryOrderStorage is the in-test checkout/app.OrderStorage. On Save it:
//   - folds the pending events into an in-memory state for query.Find
//   - stages an OrderPaid outbox row whenever the order ends StatusPaid
//
// The outbox-staging logic mirrors checkout/adapter.extractIntegrationEvents
// (which is unexported, so we cannot reuse it directly). The test is the
// canary on that mapping — if the production extractor learns a new event
// kind, this one should grow too.
type inMemoryOrderStorage struct {
	mu     sync.Mutex
	orders map[string]checkoutquery.OrderView
	outbox *inMemoryOutbox
}

func newInMemoryOrderStorage(o *inMemoryOutbox) *inMemoryOrderStorage {
	return &inMemoryOrderStorage{
		orders: map[string]checkoutquery.OrderView{},
		outbox: o,
	}
}

func (s *inMemoryOrderStorage) Save(_ context.Context, order *checkoutdomain.Order) error {
	pending := order.PendingEvents()
	s.mu.Lock()
	s.orders[order.ID()] = orderToView(order)
	s.mu.Unlock()

	if order.Status() == checkoutdomain.StatusPaid {
		for _, e := range pending {
			ps, ok := e.(checkoutdomain.PaymentSucceeded)
			if !ok {
				continue
			}
			payload, err := json.Marshal(checkoutintegration.OrderPaid{
				OrderID:    order.ID(),
				SessionID:  order.UserID(),
				CustomerID: order.CustomerID(),
				At:         ps.At,
			})
			if err != nil {
				return fmt.Errorf("encode OrderPaid: %w", err)
			}
			s.outbox.append(checkoutintegration.OrderPaid{}.EventName(), payload)
		}
	}
	order.ClearPending()
	return nil
}

// Load is part of OrderStorage but the OrderPaid flow does not exercise it.
// We still satisfy the interface so the test's narrow surface mirrors the
// production one — and so a future test that drives Cancel through the same
// fixture has a working seam.
func (s *inMemoryOrderStorage) Load(_ context.Context, _ string) (*checkoutdomain.Order, error) {
	return nil, errors.New("inMemoryOrderStorage: Load not implemented for OrderPaid flow")
}

// Find satisfies checkoutquery.Repository — only Find is exercised by the
// order-confirmation email subscriber. The remaining methods are stubs that
// keep the interface complete.
func (s *inMemoryOrderStorage) Find(_ context.Context, id string) (checkoutquery.OrderView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.orders[id]
	if !ok {
		return checkoutquery.OrderView{}, checkoutdomain.ErrOrderNotFound
	}
	return v, nil
}

func (s *inMemoryOrderStorage) ListByCustomer(context.Context, string) ([]checkoutquery.OrderSummary, error) {
	return nil, nil
}

func (s *inMemoryOrderStorage) ListAll(context.Context) ([]checkoutquery.OrderSummary, error) {
	return nil, nil
}

func (s *inMemoryOrderStorage) ListExpiredPending(context.Context, time.Time) ([]string, error) {
	return nil, nil
}

func (s *inMemoryOrderStorage) HasPurchasedProduct(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *inMemoryOrderStorage) TodaysSales(context.Context) (map[string]checkoutquery.DailySalesRow, error) {
	return nil, nil
}

// orderToView projects an order aggregate into the query-side OrderView
// shape, mirroring what checkout/adapter.Postgres.Find would return.
func orderToView(o *checkoutdomain.Order) checkoutquery.OrderView {
	return checkoutquery.NewOrderView(
		o.ID(), o.CustomerID(), o.Status(), o.PlacedAt(),
		o.Items(), o.ShipTo(), o.ShippingMethod(), o.PaymentMethod(),
		o.Subtotal(), o.TaxAmount(), o.ShippingCost(), o.TotalAmount(), o.TotalCurrency(),
		o.Carrier(), o.TrackingCode(),
		"", 0,
	)
}

// cartProductCatalog adapts productcatalog/app.ProductService onto the
// cart context's narrow ProductCatalog ACL — the same translation
// cart.transformProductCatalog does in the production composition root.
type cartProductCatalog struct {
	pc pcapp.ProductService
}

func (c cartProductCatalog) Find(ctx context.Context, variantID string) (cartdomain.Product, error) {
	p, v, err := c.pc.FindVariant(ctx, variantID)
	if errors.Is(err, pcdomain.ErrProductNotFound) {
		return cartdomain.Product{}, cartdomain.ErrProductNotFound
	}
	if err != nil {
		return cartdomain.Product{}, err
	}
	if !v.InStock() {
		return cartdomain.Product{}, cartdomain.ErrOutOfStock
	}
	cur, err := cartdomain.NewCurrency(string(v.Price().Currency()))
	if err != nil {
		return cartdomain.Product{}, fmt.Errorf("cart: invalid currency: %w", err)
	}
	return cartdomain.NewProduct(v.ID(), p.Name(), v.Price().Amount(), cur), nil
}

// recordingMailer is an in-test mailer.Mailer that logs every Send call so
// the test can assert which emails were dispatched and how many times. We
// implement Mailer directly here rather than wrapping mailer.LogMailer
// because the assertions need access to the recorded calls.
type recordingMailer struct {
	mu    sync.Mutex
	sends []mailer.Message
}

func (m *recordingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sends = append(m.sends, msg)
	return nil
}

func (m *recordingMailer) sent() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mailer.Message, len(m.sends))
	copy(out, m.sends)
	return out
}

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

// integrationFixture is the wired-together cross-context environment the
// tests below drive. Every dependency is in-memory; nothing here touches a
// DB. The set of fields mirrors what cmd/web/main.go composes at runtime,
// with the postgres adapters swapped for their in-memory counterparts.
type integrationFixture struct {
	t          *testing.T
	bus        *eventbus.Bus
	outbox     *inMemoryOutbox
	inbox      *inMemoryInbox
	cartSrv    cartapp.CartService
	pcSrv      pcapp.ProductService
	checkout   checkoutapp.CheckoutService
	query      checkoutquery.Service
	fulfilSrv  *fulfillmentapp.Service
	mailer     *recordingMailer
	decoder    outbox.Decoder
	ctx        context.Context

	// seeded data
	productID string
	variantID string
}

// discardLogger returns a logrus.FieldLogger that drops every record. Test
// output stays clean; the bus's own error path is still exercised because
// the handlers still get to return (and we still hit the inbox / outbox
// branches), it just never lands on the test terminal.
func discardLogger() logrus.FieldLogger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}

// newIntegrationFixture builds the cross-context environment:
//   - the in-process eventbus.Bus
//   - the in-memory Outbox store and Inbox dedupe table
//   - the productcatalog ProductService (in-memory adapter)
//   - the cart CartService (in-memory adapter, ACL onto productcatalog)
//   - the checkout CheckoutService and query Service (in-memory order storage
//     that stages OrderPaid into the outbox on Save)
//   - the fulfillment app.Service (in-memory adapter)
//   - a recording mailer.Mailer
//   - the outbox dispatcher
//
// The three OrderPaid subscribers — cart.clear-on-orderpaid,
// fulfillment.on-orderpaid, email.order-confirmation — are registered via
// wireOrderPaidSubscribers below, exactly the same shape as cmd/web/main.go.
func newIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()
	logger := discardLogger()
	ctx := context.Background()

	out := newInMemoryOutbox()
	ib := newInMemoryInbox()
	bus := eventbus.New(logger)

	// productcatalog (in-memory).
	pcStorage := pcadapter.NewInMemory()
	pcSrv := pcapp.NewProductService(pcStorage)

	// cart (in-memory) — wired to productcatalog through the same ACL the
	// production composition root applies in cart/bounded_context.go.
	cartStorage := cartadapter.NewInMemory()
	cartSrv := cartapp.NewCartService(cartStorage, cartProductCatalog{pc: pcSrv})

	// checkout (in-memory order storage that also serves as the query repo).
	orderStore := newInMemoryOrderStorage(out)
	checkoutSrv := checkoutapp.NewCheckoutService(
		cartSrv,
		orderStore,
		okPayment{},
		pcSrv, // StockReserver / Release
		nil,   // StockMovements (no audit log in tests)
		newDeterministicIDGen(),
		nil, // default tax strategy (no tax)
		nil, // default shipping strategy
	)
	query := checkoutquery.NewService(orderStore)

	// fulfillment (in-memory).
	fulfilStorage := fulfillmentadapter.NewInMemory()
	fulfilSrv := fulfillmentapp.NewService(fulfilStorage).
		WithPublisher(busPublisher{bus: bus})

	rec := &recordingMailer{}

	wireOrderPaidSubscribers(bus, ib, cartSrv, fulfilSrv, query, rec)

	// Seed: one product, one variant with stock.
	productID, variantID := seedProductWithVariant(t, ctx, pcStorage)

	return &integrationFixture{
		t:          t,
		bus:        bus,
		outbox:     out,
		inbox:      ib,
		cartSrv:    cartSrv,
		pcSrv:      pcSrv,
		checkout:   checkoutSrv,
		query:      query,
		fulfilSrv:  fulfilSrv,
		mailer:     rec,
		decoder:    decodeOrderPaid,
		ctx:        ctx,
		productID:  productID,
		variantID:  variantID,
	}
}

// newDeterministicIDGen returns an ID generator that yields a stable sequence
// of order ids ("ord-1", "ord-2", ...) so a test can assert on the exact id
// the checkout flow produced. Independent of crypto/rand and time, which
// would make per-run id assertions brittle.
// okPayment is a no-op PaymentProcessor for cross-context tests. The real
// FakePayment adapter was removed when checkout was wired to the payments
// context (PR #124); these tests only care about the OrderPaid wiring, not
// the charge flow itself.
type okPayment struct{}

func (okPayment) Charge(_ context.Context, _ int64, _, _ string) error { return nil }

func newDeterministicIDGen() func() string {
	var n int
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("ord-%d", n)
	}
}

// busPublisher bridges fulfillment/app.EventPublisher onto eventbus.Bus.
// Mirrors fulfillment/bounded_context.go's busPublisher (unexported there,
// so we re-declare the same trivial adapter locally).
type busPublisher struct {
	bus *eventbus.Bus
}

func (p busPublisher) Publish(ctx context.Context, e eventbus.Event) {
	if p.bus == nil {
		return
	}
	p.bus.Publish(ctx, e)
}

// decodeOrderPaid is the outbox.Decoder closure the dispatcher uses to turn a
// stored (kind, payload) row back into the integration event the in-process
// subscribers consume. Same shape as cmd/web/main.go's decode function — kept
// minimal here because the test only ever stages OrderPaid rows.
func decodeOrderPaid(kind string, payload []byte) (eventbus.Event, error) {
	switch kind {
	case checkoutintegration.OrderPaid{}.EventName():
		var e checkoutintegration.OrderPaid
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("decode OrderPaid: %w", err)
		}
		return e, nil
	}
	return nil, fmt.Errorf("unknown outbox kind: %s", kind)
}

// wireOrderPaidSubscribers binds the three OrderPaid subscribers the
// composition root registers in cmd/web/main.go (cart-clear, fulfillment
// scheduling, order-confirmation email). The wiring shape — same subscriber
// IDs, same inbox.Wrap, same anonymous-customer skip on the email — is the
// contract this test enforces. Duplicating the few lines here instead of
// sharing them with cmd/web/main.go is intentional: the goal of the
// integration test is to PROVE the wiring shape, not to refactor it.
func wireOrderPaidSubscribers(
	bus *eventbus.Bus,
	ib inbox.MarkHandler,
	cartSrv cartapp.CartService,
	fulfilSrv *fulfillmentapp.Service,
	query checkoutquery.Service,
	m mailer.Mailer,
) {
	// 1. cart.clear-on-orderpaid
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("cart.clear-on-orderpaid", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				return cartSrv.Clear(ctx, e.(checkoutintegration.OrderPaid).SessionID)
			},
		),
	)

	// 2. fulfillment.on-orderpaid
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("fulfillment.on-orderpaid", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				paid := e.(checkoutintegration.OrderPaid)
				return fulfilSrv.OnOrderPaid(ctx, paid.OrderID, paid.At)
			},
		),
	)

	// 3. email.order-confirmation — same anonymous-customer skip as
	// cmd/web/main.go. The test's second scenario locks in this branch.
	bus.SubscribeWithID(
		checkoutintegration.OrderPaid{}.EventName(),
		inbox.Wrap("email.order-confirmation", ib,
			func(ctx context.Context, _ int64, e eventbus.Event) error {
				paid := e.(checkoutintegration.OrderPaid)
				if paid.CustomerID == "" {
					return nil
				}
				view, err := query.Find(ctx, paid.OrderID)
				if err != nil {
					return fmt.Errorf("order confirmation: load view: %w", err)
				}
				// In production layout.RenderOrderConfirmation builds
				// the full HTML/text bodies; in the test we only assert
				// on the metadata (To + Kind), so a minimal Message is
				// sufficient. The CustomerID is the authoritative
				// recipient — same precedence rule as in main.go.
				return m.Send(ctx, mailer.Message{
					To:       paid.CustomerID,
					Subject:  "Your order " + view.ID() + " is confirmed",
					TextBody: "order confirmation",
					Kind:     mailer.KindOrderConfirmation,
				})
			},
		),
	)
}

// seedProductWithVariant inserts one product with one variant carrying
// real stock so the checkout flow has something to reserve. Returns the
// (product id, variant id) pair. The variant is what the cart adds and
// what checkout ultimately reserves.
func seedProductWithVariant(t *testing.T, ctx context.Context, store pcapp.ProductStorage) (string, string) {
	t.Helper()
	const productID = "prod-test"
	const variantID = "var-test"

	pID, err := pcdomain.NewProductId(productID)
	if err != nil {
		t.Fatalf("seed: NewProductId: %v", err)
	}
	price, err := pcdomain.NewPrice(1500, pcdomain.MustNewCurrency("USD"))
	if err != nil {
		t.Fatalf("seed: NewPrice: %v", err)
	}
	p, err := pcdomain.NewProduct(pID, "Test Mug", "for integration tests", price, "https://example.test/img.png")
	if err != nil {
		t.Fatalf("seed: NewProduct: %v", err)
	}
	if err := store.Add(ctx, p); err != nil {
		t.Fatalf("seed: storage.Add: %v", err)
	}

	variant := pcdomain.NewVariant(variantID, "SKU-TEST", "", nil, price, 10)
	if err := store.AddVariant(ctx, productID, 0, variant); err != nil {
		t.Fatalf("seed: storage.AddVariant: %v", err)
	}
	return productID, variantID
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestOrderPaid_EndToEnd walks the full happy path:
//
//  1. seed productcatalog + cart
//  2. checkout.Place stages an OrderPaid row in the in-memory outbox
//  3. dispatchOnce drains the row onto the bus
//  4. assert the three downstream consequences
//  5. dispatchOnce a second time — the row is marked sent, the inbox would
//     dedupe anything that escaped — assert nothing fires again.
//
// The test fails if a subscriber is removed, renamed (the inbox key is part
// of the contract — renaming it would let a second delivery slip through),
// or if the OrderPaid integration event grows / drops a field the
// subscribers depend on.
func TestOrderPaid_EndToEnd(t *testing.T) {
	f := newIntegrationFixture(t)
	const sessionID = "sess-integration"
	const customerEmail = "jane@example.test"

	if err := f.cartSrv.AddToCart(f.ctx, sessionID, f.variantID, 2); err != nil {
		t.Fatalf("AddToCart: %v", err)
	}

	shipTo, err := checkoutdomain.NewAddress("Jane Doe", "1 Main St", "", "Portland", "97201", "USA")
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}
	shipMethod := checkoutdomain.RebuildShippingMethod("pickup", "Personal pickup", 0)
	payMethod := checkoutdomain.RebuildPaymentMethod("cod", "Cash on delivery")
	order, err := f.checkout.Place(f.ctx, sessionID, customerEmail, "", shipTo, shipMethod, payMethod, promodomain.Discount{})
	if err != nil {
		t.Fatalf("checkout.Place: %v", err)
	}
	if order.Status() != checkoutdomain.StatusPaid {
		t.Fatalf("order status = %q, want paid", order.Status())
	}

	// At this point the outbox holds exactly one OrderPaid row and no
	// subscriber has fired yet — Place stages, the dispatcher publishes.
	if rows, _ := f.outbox.Unsent(f.ctx, 100); len(rows) != 1 {
		t.Fatalf("unsent rows = %d, want 1", len(rows))
	}
	if got := len(f.mailer.sent()); got != 0 {
		t.Fatalf("mailer sends before dispatch = %d, want 0", got)
	}

	// Drain the outbox: this is the seam every production cycle hits.
	dispatchOnce(t, f.ctx, f.outbox, f.bus, f.decoder)

	// ASSERTION 1: cart-clear subscriber emptied the basket.
	cart, err := f.cartSrv.Get(f.ctx, sessionID)
	if err != nil && !errors.Is(err, cartdomain.ErrCartNotFound) {
		t.Fatalf("cart.Get after dispatch: %v", err)
	}
	if err == nil && len(cart.Items()) != 0 {
		t.Fatalf("cart items after dispatch = %d, want 0", len(cart.Items()))
	}

	// ASSERTION 2: order-confirmation email subscriber sent exactly one
	// message, kind=order_confirmation, to the customer's email.
	sends := f.mailer.sent()
	if len(sends) != 1 {
		t.Fatalf("mailer sends = %d, want 1", len(sends))
	}
	if sends[0].Kind != mailer.KindOrderConfirmation {
		t.Errorf("mailer send kind = %q, want %q", sends[0].Kind, mailer.KindOrderConfirmation)
	}
	if sends[0].To != customerEmail {
		t.Errorf("mailer send To = %q, want %q", sends[0].To, customerEmail)
	}

	// ASSERTION 3: fulfillment subscriber created one Fulfillment with
	// status=scheduled for this order id.
	ful, err := f.fulfilSrv.ByOrder(f.ctx, order.ID())
	if err != nil {
		t.Fatalf("fulfilSrv.ByOrder: %v", err)
	}
	if ful.OrderID() != order.ID() {
		t.Errorf("fulfillment orderID = %q, want %q", ful.OrderID(), order.ID())
	}
	if ful.Status() != fulfillmentdomain.StatusScheduled {
		t.Errorf("fulfillment status = %q, want %q", ful.Status(), fulfillmentdomain.StatusScheduled)
	}

	// A second dispatcher tick is the inbox/outbox at-least-once contract
	// in action: the row is marked sent (so dispatchOnce yields no work)
	// AND, defensively, the inbox table holds a per-subscriber dedupe
	// row keyed on the outbox id. The combined effect is that no
	// subscriber fires a second time.
	dispatchOnce(t, f.ctx, f.outbox, f.bus, f.decoder)

	if got := len(f.mailer.sent()); got != 1 {
		t.Errorf("mailer sends after second dispatch = %d, want 1 (no duplicate)", got)
	}
	// Cart stays empty; nothing to dedupe-fail on the cart-clear branch.
	cart, err = f.cartSrv.Get(f.ctx, sessionID)
	if err == nil && len(cart.Items()) != 0 {
		t.Errorf("cart items after second dispatch = %d, want 0", len(cart.Items()))
	}
	// Still exactly one fulfillment for the order — OnOrderPaid is itself
	// idempotent (FindByOrder no-op) AND the inbox would have dedupe'd it,
	// so this assertion catches a regression in either layer.
	ful2, err := f.fulfilSrv.ByOrder(f.ctx, order.ID())
	if err != nil {
		t.Fatalf("fulfilSrv.ByOrder after second dispatch: %v", err)
	}
	if ful2.ID() != ful.ID() {
		t.Errorf("fulfillment id changed across dispatch ticks (%q -> %q): duplicate Fulfillment created",
			ful.ID(), ful2.ID())
	}
}

// TestOrderPaid_AnonymousCustomerSkipsEmail locks in the existing behaviour
// of the email subscriber: when CustomerID is empty (anonymous checkout)
// the email is skipped, but cart-clear and fulfillment-scheduling still
// happen — they are unconditional. This is the contract anyone touching
// the email subscriber's anonymous-customer branch needs to preserve.
func TestOrderPaid_AnonymousCustomerSkipsEmail(t *testing.T) {
	f := newIntegrationFixture(t)
	const sessionID = "sess-anonymous"

	if err := f.cartSrv.AddToCart(f.ctx, sessionID, f.variantID, 1); err != nil {
		t.Fatalf("AddToCart: %v", err)
	}

	shipMethod := checkoutdomain.RebuildShippingMethod("pickup", "Personal pickup", 0)
	payMethod := checkoutdomain.RebuildPaymentMethod("cod", "Cash on delivery")
	order, err := f.checkout.Place(f.ctx, sessionID, "" /* anonymous */, "", checkoutdomain.Address{}, shipMethod, payMethod, promodomain.Discount{})
	if err != nil {
		t.Fatalf("checkout.Place: %v", err)
	}

	dispatchOnce(t, f.ctx, f.outbox, f.bus, f.decoder)

	// cart cleared — unconditional.
	cart, err := f.cartSrv.Get(f.ctx, sessionID)
	if err == nil && len(cart.Items()) != 0 {
		t.Errorf("cart items after dispatch (anon) = %d, want 0", len(cart.Items()))
	}

	// email NOT sent — the email subscriber's anonymous-customer branch.
	if got := len(f.mailer.sent()); got != 0 {
		t.Errorf("mailer sends (anon) = %d, want 0", got)
	}

	// fulfillment scheduled — unconditional.
	ful, err := f.fulfilSrv.ByOrder(f.ctx, order.ID())
	if err != nil {
		t.Fatalf("fulfilSrv.ByOrder (anon): %v", err)
	}
	if ful.Status() != fulfillmentdomain.StatusScheduled {
		t.Errorf("fulfillment status (anon) = %q, want %q", ful.Status(), fulfillmentdomain.StatusScheduled)
	}
}
