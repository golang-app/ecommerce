package app_test

import (
	"context"
	"errors"
	"testing"

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
	return f.saved, nil
}

type fakePaymentProcessor struct {
	chargeErr error
}

func (f *fakePaymentProcessor) Charge(ctx context.Context, amount int64, currency, cardNumber string) error {
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
	f.released = quantities
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
