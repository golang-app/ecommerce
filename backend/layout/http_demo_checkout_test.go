package layout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type mockDemoCheckoutQry struct {
	order checkoutQuery.OrderView
}

func (m mockDemoCheckoutQry) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	return m.order, nil
}
func (m mockDemoCheckoutQry) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}
func (m mockDemoCheckoutQry) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}
func (m mockDemoCheckoutQry) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}
func (m mockDemoCheckoutQry) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}
func (m mockDemoCheckoutQry) ListCustomerOrderStats(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
	return nil, nil
}

func TestDemoCheckout_PaymentAndEmailNotice(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{methods: methods}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)

	// Demo Mode ON
	handlerDemo := httpHandler{
		cartSrv:     cartSrv,
		checkoutSrv: mockCmds,
		rates:       rates,
		logger:      logger,
		demoMode:    true,
	}

	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()
	handlerDemo.Checkout(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "demo-checkout-notice"))
	is.True(strings.Contains(body, "No real payment is processed"))
	is.True(strings.Contains(body, "Order confirmation emails are simulated"))

	// Demo Mode OFF
	handlerNoDemo := httpHandler{
		cartSrv:     cartSrv,
		checkoutSrv: mockCmds,
		rates:       rates,
		logger:      logger,
		demoMode:    false,
	}

	req2 := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	req2.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec2 := httptest.NewRecorder()
	handlerNoDemo.Checkout(rec2, req2)

	is.Equal(rec2.Code, http.StatusOK)
	body2 := rec2.Body.String()
	is.True(!strings.Contains(body2, "demo-checkout-notice"))
}

func TestDemoOrder_Notice(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)

	orderView := checkoutQuery.NewOrderView(
		"ord-123", "cust-1", checkoutDomain.StatusPaid, time.Now(),
		nil, checkoutDomain.Address{}, checkoutDomain.ShippingMethod{}, checkoutDomain.PaymentMethod{},
		5000, 0, 0, 5000, "USD",
		"Standard Post", "TRACK123",
		"", 0,
	)

	// Demo Mode ON
	handlerDemo := httpHandler{
		checkoutQry: mockDemoCheckoutQry{order: orderView},
		rates:       rates,
		logger:      logger,
		demoMode:    true,
	}

	req := httptest.NewRequest(http.MethodGet, "/order/ord-123", nil)
	req = mux.SetURLVars(req, map[string]string{"orderID": "ord-123"})
	rec := httptest.NewRecorder()
	handlerDemo.Order(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "demo-order-notice"))
	is.True(strings.Contains(body, "This order is simulated"))

	// Demo Mode OFF
	handlerNoDemo := httpHandler{
		checkoutQry: mockDemoCheckoutQry{order: orderView},
		rates:       rates,
		logger:      logger,
		demoMode:    false,
	}

	req2 := httptest.NewRequest(http.MethodGet, "/order/ord-123", nil)
	req2 = mux.SetURLVars(req2, map[string]string{"orderID": "ord-123"})
	rec2 := httptest.NewRecorder()
	handlerNoDemo.Order(rec2, req2)

	is.Equal(rec2.Code, http.StatusOK)
	body2 := rec2.Body.String()
	is.True(!strings.Contains(body2, "demo-order-notice"))
}
