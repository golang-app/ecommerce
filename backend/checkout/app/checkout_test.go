package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/sirupsen/logrus"
)

type logCollector struct {
	entries []*logrus.Entry
}

func (c *logCollector) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (c *logCollector) Fire(e *logrus.Entry) error {
	c.entries = append(c.entries, e)
	return nil
}

type fakeCartReader struct {
	cart *cartDomain.Cart
	err  error
}

func (f *fakeCartReader) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cart, nil
}

type fakeOrderStorage struct {
	saved *domain.Order
}

func (f *fakeOrderStorage) Save(ctx context.Context, order *domain.Order) error {
	f.saved = order
	return nil
}

func (f *fakeOrderStorage) Load(ctx context.Context, id string) (*domain.Order, error) {
	if f.saved != nil && f.saved.ID() == id {
		return f.saved, nil
	}
	return nil, domain.ErrOrderNotFound
}

type fakePaymentProcessor struct {
	chargeErr error
	called    bool
}

func (f *fakePaymentProcessor) Charge(ctx context.Context, amount int64, currency, cardNumber string) error {
	f.called = true
	return f.chargeErr
}

type fakeStockReserver struct {
	reserved map[string]int
	released map[string]int
}

func (f *fakeStockReserver) Reserve(ctx context.Context, quantities map[string]int) error {
	f.reserved = quantities
	return nil
}

func (f *fakeStockReserver) Release(ctx context.Context, quantities map[string]int) error {
	if f.released == nil {
		f.released = make(map[string]int)
	}
	for k, v := range quantities {
		f.released[k] += v
	}
	return nil
}

type fakePromoRedeemer struct {
	redeemedCode string
}

func (f *fakePromoRedeemer) Redeem(ctx context.Context, code, orderID, customerID string, discount promodomain.Discount) error {
	f.redeemedCode = code
	return nil
}

func TestPlaceOrder_LogsKeyStages(t *testing.T) {
	collector := &logCollector{}
	logger := logrus.New()
	logger.AddHook(collector)

	ctx := observability.WithLogger(context.Background(), logger)

	cart := cartDomain.NewCart(cartDomain.NewUser("cust-1"))
	p := cartDomain.NewProduct("prod-1", "Shoes", 5000, cartDomain.MustNewCurrency("USD"))
	if err := cart.Add(p, 2); err != nil {
		t.Fatalf("cart.Add: %v", err)
	}

	cartReader := &fakeCartReader{cart: cart}
	storage := &fakeOrderStorage{}
	payment := &fakePaymentProcessor{}
	stock := &fakeStockReserver{}

	svc := app.NewCheckoutService(
		cartReader,
		storage,
		payment,
		stock,
		nil,
		func() string { return "order-test-123" },
		domain.FlatTaxStrategy{},
		domain.ThresholdShippingStrategy{},
	)

	addr, err := domain.NewAddress("Jane Doe", "Main 1", "", "City", "10001", "USA")
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}

	shipMethod, err := domain.ShippingMethodByCode("courier")
	if err != nil {
		t.Fatalf("ShippingMethodByCode: %v", err)
	}

	payMethod, err := domain.PaymentMethodByCode("card")
	if err != nil {
		t.Fatalf("PaymentMethodByCode: %v", err)
	}

	order, err := svc.Place(ctx, "sess-1", "cust-1", "1234", addr, shipMethod, payMethod, promodomain.Discount{})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}

	if order.ID() != "order-test-123" {
		t.Errorf("expected order ID 'order-test-123', got %q", order.ID())
	}

	expectedLogs := []string{
		"Starting checkout order processing",
		"Cart retrieved with items for checkout",
		"Stock reserved successfully for order",
		"Order aggregate created and price quote calculated",
		"Attempting payment charge",
		"Payment charge approved",
		"Order placed and finalized successfully",
	}

	logMessages := make(map[string]bool)
	for _, entry := range collector.entries {
		logMessages[entry.Message] = true
	}

	for _, expected := range expectedLogs {
		if !logMessages[expected] {
			t.Errorf("expected log message %q was not found in logged entries", expected)
		}
	}
}

func TestPlaceOrder_LogsPaymentFailure(t *testing.T) {
	collector := &logCollector{}
	logger := logrus.New()
	logger.AddHook(collector)

	ctx := observability.WithLogger(context.Background(), logger)

	cart := cartDomain.NewCart(cartDomain.NewUser("cust-1"))
	p := cartDomain.NewProduct("prod-1", "Shoes", 5000, cartDomain.MustNewCurrency("USD"))
	if err := cart.Add(p, 1); err != nil {
		t.Fatalf("cart.Add: %v", err)
	}

	cartReader := &fakeCartReader{cart: cart}
	storage := &fakeOrderStorage{}
	payment := &fakePaymentProcessor{chargeErr: errors.New("insufficient funds")}
	stock := &fakeStockReserver{}

	svc := app.NewCheckoutService(
		cartReader,
		storage,
		payment,
		stock,
		nil,
		func() string { return "order-test-failed" },
		domain.FlatTaxStrategy{},
		domain.ThresholdShippingStrategy{},
	)

	addr, _ := domain.NewAddress("Jane Doe", "Main 1", "", "City", "10001", "USA")
	shipMethod, _ := domain.ShippingMethodByCode("courier")
	payMethod, _ := domain.PaymentMethodByCode("card")

	_, err := svc.Place(ctx, "sess-1", "cust-1", "1234", addr, shipMethod, payMethod, promodomain.Discount{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	expectedDeclinedMsg := "Payment charge declined, releasing reserved stock"
	found := false
	for _, entry := range collector.entries {
		if entry.Message == expectedDeclinedMsg {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected log %q when payment is declined", expectedDeclinedMsg)
	}
}

func TestMarkPaid_TransitionsToPaid(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	stock := &fakeStockReserver{}
	svc := app.NewCheckoutService(
		nil,
		storage,
		nil,
		stock,
		nil,
		nil,
		nil,
		nil,
	)

	lines := []domain.Line{
		domain.NewLine("prod-1", "Shoes", 2, 5000, "USD"),
	}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, err := domain.PlaceOrder("order-123", "sess-1", "cust-1", domain.Address{},
		method,
		domain.RebuildPaymentMethod("fake", "Fake Payment Simulator"),
		lines, 0, method.Cost(), "", 0, "web", time.Now())
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	storage.saved = order

	// MarkPaid should transition to paid
	if err := svc.MarkPaid(ctx, "order-123"); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}

	if storage.saved.Status() != domain.StatusPaid {
		t.Errorf("expected status %s, got %s", domain.StatusPaid, storage.saved.Status())
	}

	// Calling MarkPaid again is idempotent (returns nil without error)
	if err := svc.MarkPaid(ctx, "order-123"); err != nil {
		t.Fatalf("MarkPaid idempotent call failed: %v", err)
	}
	if storage.saved.Status() != domain.StatusPaid {
		t.Errorf("expected status %s, got %s", domain.StatusPaid, storage.saved.Status())
	}
}

func TestMarkPaymentFailed_ReleasesStockAndFailsOrder(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	stock := &fakeStockReserver{}
	svc := app.NewCheckoutService(
		nil,
		storage,
		nil,
		stock,
		nil,
		nil,
		nil,
		nil,
	)

	lines := []domain.Line{
		domain.NewLine("prod-1", "Shoes", 2, 5000, "USD"),
	}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, err := domain.PlaceOrder("order-123", "sess-1", "cust-1", domain.Address{},
		method,
		domain.RebuildPaymentMethod("fake", "Fake Payment Simulator"),
		lines, 0, method.Cost(), "", 0, "web", time.Now())
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}

	storage.saved = order

	if err := svc.MarkPaymentFailed(ctx, "order-123", "insufficient funds"); err != nil {
		t.Fatalf("MarkPaymentFailed: %v", err)
	}

	if storage.saved.Status() != domain.StatusFailed {
		t.Errorf("expected status %s, got %s", domain.StatusFailed, storage.saved.Status())
	}

	if stock.released["prod-1"] != 2 {
		t.Errorf("expected stock release of 2 for prod-1, got %d", stock.released["prod-1"])
	}

	// Calling again is idempotent
	if err := svc.MarkPaymentFailed(ctx, "order-123", "insufficient funds"); err != nil {
		t.Fatalf("MarkPaymentFailed idempotent call failed: %v", err)
	}
	if storage.saved.Status() != domain.StatusFailed {
		t.Errorf("expected status %s, got %s", domain.StatusFailed, storage.saved.Status())
	}
}

func TestPlace_FakePaymentLeavesOrderPending(t *testing.T) {
	ctx := context.Background()

	cart := cartDomain.NewCart(cartDomain.NewUser("cust-1"))
	p := cartDomain.NewProduct("prod-1", "Shoes", 5000, cartDomain.MustNewCurrency("USD"))
	if err := cart.Add(p, 2); err != nil {
		t.Fatalf("cart.Add: %v", err)
	}

	cartReader := &fakeCartReader{cart: cart}
	storage := &fakeOrderStorage{}
	payment := &fakePaymentProcessor{}
	stock := &fakeStockReserver{}

	svc := app.NewCheckoutService(
		cartReader,
		storage,
		payment,
		stock,
		nil,
		func() string { return "order-fake-123" },
		domain.FlatTaxStrategy{},
		domain.ThresholdShippingStrategy{},
	)

	addr, err := domain.NewAddress("Jane Doe", "Main 1", "", "City", "10001", "USA")
	if err != nil {
		t.Fatalf("NewAddress: %v", err)
	}
	shipMethod, err := domain.ShippingMethodByCode("courier")
	if err != nil {
		t.Fatalf("ShippingMethodByCode: %v", err)
	}
	payMethod, err := domain.PaymentMethodByCode("fake")
	if err != nil {
		t.Fatalf("PaymentMethodByCode: %v", err)
	}

	order, err := svc.Place(ctx, "sess-1", "cust-1", "", addr, shipMethod, payMethod, promodomain.Discount{})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}

	if order.Status() != domain.StatusPending {
		t.Errorf("expected returned order status %s, got %s", domain.StatusPending, order.Status())
	}

	if storage.saved == nil {
		t.Fatal("expected order to be saved in storage, got nil")
	}
	if storage.saved.Status() != domain.StatusPending {
		t.Errorf("expected stored order status %s, got %s", domain.StatusPending, storage.saved.Status())
	}

	if payment.called {
		t.Error("expected payment processor NOT to be called for fake payment method")
	}

	if stock.reserved["prod-1"] != 2 {
		t.Errorf("expected stock reserved for prod-1 = 2, got %d", stock.reserved["prod-1"])
	}
	if len(stock.released) != 0 {
		t.Errorf("expected no stock to be released, got %v", stock.released)
	}
}

func TestMarkPaid_NotPendingFails(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	svc := app.NewCheckoutService(nil, storage, nil, nil, nil, nil, nil, nil)

	lines := []domain.Line{domain.NewLine("prod-1", "Shoes", 1, 5000, "USD")}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, _ := domain.PlaceOrder("order-1", "sess-1", "cust-1", domain.Address{},
		method, domain.RebuildPaymentMethod("fake", "Fake"), lines, 0, 1500, "", 0, "web", time.Now())
	order.MarkFailed("declined", time.Now())
	storage.saved = order

	err := svc.MarkPaid(ctx, "order-1")
	if err == nil || err.Error() != "order is not pending" {
		t.Fatalf("expected 'order is not pending', got %v", err)
	}
}

func TestMarkPaid_RedeemsPromo(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	promo := &fakePromoRedeemer{}
	svc := app.NewCheckoutService(nil, storage, nil, nil, nil, nil, nil, nil).WithPromoRedeemer(promo)

	lines := []domain.Line{domain.NewLine("prod-1", "Shoes", 1, 5000, "USD")}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, _ := domain.PlaceOrder("order-1", "sess-1", "cust-1", domain.Address{},
		method, domain.RebuildPaymentMethod("fake", "Fake"), lines, 0, 1500, "PROMO10", 500, "web", time.Now())
	storage.saved = order

	if err := svc.MarkPaid(ctx, "order-1"); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if promo.redeemedCode != "PROMO10" {
		t.Errorf("expected promo code 'PROMO10' redeemed, got %q", promo.redeemedCode)
	}
}

func TestMarkPaymentFailed_NotPendingFails(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	svc := app.NewCheckoutService(nil, storage, nil, nil, nil, nil, nil, nil)

	lines := []domain.Line{domain.NewLine("prod-1", "Shoes", 1, 5000, "USD")}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, _ := domain.PlaceOrder("order-1", "sess-1", "cust-1", domain.Address{},
		method, domain.RebuildPaymentMethod("fake", "Fake"), lines, 0, 1500, "", 0, "web", time.Now())
	order.MarkPaid(time.Now())
	storage.saved = order

	err := svc.MarkPaymentFailed(ctx, "order-1", "too late")
	if err == nil || err.Error() != "order is not pending" {
		t.Fatalf("expected 'order is not pending', got %v", err)
	}
}

func TestMarkPaymentFailed_DefaultReason(t *testing.T) {
	ctx := context.Background()
	storage := &fakeOrderStorage{}
	stock := &fakeStockReserver{}
	svc := app.NewCheckoutService(nil, storage, nil, stock, nil, nil, nil, nil)

	lines := []domain.Line{domain.NewLine("prod-1", "Shoes", 1, 5000, "USD")}
	method := domain.RebuildShippingMethod("courier", "Courier", 1500)
	order, _ := domain.PlaceOrder("order-1", "sess-1", "cust-1", domain.Address{},
		method, domain.RebuildPaymentMethod("fake", "Fake"), lines, 0, 1500, "", 0, "web", time.Now())
	storage.saved = order

	if err := svc.MarkPaymentFailed(ctx, "order-1", ""); err != nil {
		t.Fatalf("MarkPaymentFailed: %v", err)
	}
	if storage.saved.Status() != domain.StatusFailed {
		t.Errorf("expected status %s, got %s", domain.StatusFailed, storage.saved.Status())
	}
}


