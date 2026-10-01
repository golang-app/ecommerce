package layout

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	paymentsAdapter "github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	paymentsApp "github.com/bkielbasa/go-ecommerce/backend/payments/app"
	paymentsDomain "github.com/bkielbasa/go-ecommerce/backend/payments/domain"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// e2eCheckoutService implements both checkoutCommands and checkoutQueries
// to maintain an in-memory lifecycle of orders for end-to-end integration tests.
type e2eCheckoutService struct {
	mu           sync.Mutex
	orders       map[string]*checkoutDomain.Order
	orderViews   map[string]checkoutQuery.OrderView
	paidOrders   []string
	failedOrders map[string]string
	nextID       int
}

func newE2ECheckoutService() *e2eCheckoutService {
	return &e2eCheckoutService{
		orders:       make(map[string]*checkoutDomain.Order),
		orderViews:   make(map[string]checkoutQuery.OrderView),
		failedOrders: make(map[string]string),
	}
}

func (s *e2eCheckoutService) Place(
	ctx context.Context,
	sessID, customerID, cardNumber string,
	shipTo checkoutDomain.Address,
	shipMethod checkoutDomain.ShippingMethod,
	payMethod checkoutDomain.PaymentMethod,
	discount promodomain.Discount,
) (checkoutDomain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	orderID := fmt.Sprintf("ord-e2e-%d", s.nextID)

	items := []checkoutDomain.Line{
		checkoutDomain.NewLine("prod-1", "Test Product", 1, 5000, "USD"),
	}

	order := checkoutDomain.NewOrder(
		orderID,
		sessID,
		customerID,
		shipTo,
		shipMethod,
		payMethod,
		items,
		checkoutDomain.StatusPending,
		time.Now().UTC(),
	)
	s.orders[orderID] = &order

	view := checkoutQuery.NewOrderView(
		orderID,
		customerID,
		checkoutDomain.StatusPending,
		time.Now().UTC(),
		items,
		shipTo,
		shipMethod,
		payMethod,
		5000, 0, shipMethod.Cost(), order.TotalAmount(),
		"USD", "", "", "", 0,
	)
	s.orderViews[orderID] = view

	return order, nil
}

func (s *e2eCheckoutService) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}

func (s *e2eCheckoutService) AdminCancel(ctx context.Context, orderID string) error {
	return nil
}

func (s *e2eCheckoutService) MarkPaid(ctx context.Context, orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.paidOrders = append(s.paidOrders, orderID)
	if order, ok := s.orders[orderID]; ok {
		*order = checkoutDomain.NewOrder(
			order.ID(),
			order.UserID(),
			order.CustomerID(),
			order.ShipTo(),
			order.ShippingMethod(),
			order.PaymentMethod(),
			order.Items(),
			checkoutDomain.StatusPaid,
			order.PlacedAt(),
		)
	}
	if view, ok := s.orderViews[orderID]; ok {
		s.orderViews[orderID] = checkoutQuery.NewOrderView(
			view.ID(),
			view.CustomerID(),
			checkoutDomain.StatusPaid,
			view.PlacedAt(),
			view.Items(),
			view.ShipTo(),
			view.ShippingMethod(),
			view.PaymentMethod(),
			view.Subtotal(),
			view.TaxAmount(),
			view.ShippingCost(),
			view.TotalAmount(),
			view.TotalCurrency(),
			view.Carrier(),
			view.TrackingCode(),
			view.DiscountCode(),
			view.DiscountAmount(),
		)
	}
	return nil
}

func (s *e2eCheckoutService) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.failedOrders[orderID] = reason
	if order, ok := s.orders[orderID]; ok {
		*order = checkoutDomain.NewOrder(
			order.ID(),
			order.UserID(),
			order.CustomerID(),
			order.ShipTo(),
			order.ShippingMethod(),
			order.PaymentMethod(),
			order.Items(),
			checkoutDomain.StatusFailed,
			order.PlacedAt(),
		)
	}
	if view, ok := s.orderViews[orderID]; ok {
		s.orderViews[orderID] = checkoutQuery.NewOrderView(
			view.ID(),
			view.CustomerID(),
			checkoutDomain.StatusFailed,
			view.PlacedAt(),
			view.Items(),
			view.ShipTo(),
			view.ShippingMethod(),
			view.PaymentMethod(),
			view.Subtotal(),
			view.TaxAmount(),
			view.ShippingCost(),
			view.TotalAmount(),
			view.TotalCurrency(),
			view.Carrier(),
			view.TrackingCode(),
			view.DiscountCode(),
			view.DiscountAmount(),
		)
	}
	return nil
}

func (s *e2eCheckoutService) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return checkoutDomain.ShippingMethods(), nil
}

func (s *e2eCheckoutService) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	return checkoutDomain.ShippingMethodByCode(code)
}

func (s *e2eCheckoutService) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	return nil
}

func (s *e2eCheckoutService) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	return nil
}

func (s *e2eCheckoutService) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if view, ok := s.orderViews[id]; ok {
		return view, nil
	}
	return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
}

func (s *e2eCheckoutService) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (s *e2eCheckoutService) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (s *e2eCheckoutService) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}

func (s *e2eCheckoutService) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}

// e2eCartService satisfies cartService for the test harness.
type e2eCartService struct {
	cart *cartDomain.Cart
}

func (c *e2eCartService) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	return nil
}

func (c *e2eCartService) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if c.cart != nil {
		return c.cart, nil
	}
	return nil, cartDomain.ErrCartNotFound
}

// setupE2EEnvironment initializes the full bounded context with in-memory payments storage,
// order service, cart, admin session capability, and registered mux router.
func setupE2EEnvironment(t *testing.T) (*mux.Router, *paymentsApp.Service, *e2eCheckoutService) {
	t.Helper()
	setupFakePaymentEnv(t)

	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	checkoutSrv := newE2ECheckoutService()

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &e2eCartService{cart: cart}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	bc := New(
		logger,
		cartSrv,
		nil, // catalogSrv
		nil, // authSrv
		adminAuth,
		checkoutSrv,
		checkoutSrv,
		nil, // fulfillmentSrv
		nil, // repricingSrv
		nil, // shipSrv
		nil, // reviewsSrv
		nil, // wishlistSrv
		nil, // promoSrv
		nil, // searchSrv
		nil, // storeSrv
		nil, // imageStore
		"",  // uploadsDir
		[]byte("test-secret-12345678901234567890"),
		false, // cookieSecure
		false, // csrfEnabled
		nil,   // mailerSrv
		"http://localhost:8080",
		rates,
		"",
		paySrv,
	)

	router := mux.NewRouter()
	bc.(*boundedContext).MuxRegister(router)
	return router, paySrv, checkoutSrv
}

func TestPaymentProviders_EndToEnd(t *testing.T) {
	t.Run("AdminProviderConfiguration_And_CheckoutVisibility", func(t *testing.T) {
		router, paySrv, _ := setupE2EEnvironment(t)
		ctx := context.Background()

		// Step 1: Admin lists providers and verifies both Stripe and Fake are initially present.
		reqList := httptest.NewRequest(http.MethodGet, "/admin/payment-providers", nil)
		setAdminSession(t, reqList)
		recList := httptest.NewRecorder()
		router.ServeHTTP(recList, reqList)

		if recList.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /admin/payment-providers, got %d: %s", recList.Code, recList.Body.String())
		}
		listBody := recList.Body.String()
		if !strings.Contains(listBody, "Stripe") {
			t.Errorf("expected admin provider list to contain 'Stripe'")
		}
		if !strings.Contains(listBody, "Fake Payment Simulator") {
			t.Errorf("expected admin provider list to contain 'Fake Payment Simulator'")
		}

		// Verify domain-level state is also both enabled initially.
		providers, err := paySrv.ListProviders(ctx)
		if err != nil {
			t.Fatalf("failed to list providers from payments service: %v", err)
		}
		var stripeFound, fakeFound bool
		for _, p := range providers {
			if p.ID() == paymentsDomain.ProviderStripe && p.IsEnabled() {
				stripeFound = true
			}
			if p.ID() == paymentsDomain.ProviderFake && p.IsEnabled() {
				fakeFound = true
			}
		}
		if !stripeFound || !fakeFound {
			t.Fatalf("expected both Stripe and Fake providers to be initially enabled")
		}

		// Step 2: Admin disables Fake provider -> /checkout offers only Card, PayPal, Cash on Delivery.
		formDisableFake := url.Values{
			"name":        []string{"Fake Payment Simulator"},
			"description": []string{"Interactive payment simulator for testing payment outcomes."},
			// "enabled" omitted -> disabled
		}
		reqDisableFake := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/fake", strings.NewReader(formDisableFake.Encode()))
		reqDisableFake.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setAdminSession(t, reqDisableFake)
		recDisableFake := httptest.NewRecorder()
		router.ServeHTTP(recDisableFake, reqDisableFake)

		if recDisableFake.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when disabling fake provider, got %d", recDisableFake.Code)
		}
		fakeCfg, err := paySrv.FindProvider(ctx, paymentsDomain.ProviderFake)
		if err != nil || fakeCfg.IsEnabled() {
			t.Fatalf("expected Fake provider to be disabled in service, got err=%v, enabled=%v", err, fakeCfg.IsEnabled())
		}

		// Verify /checkout with Fake disabled
		reqCheckout1 := httptest.NewRequest(http.MethodGet, "/checkout", nil)
		reqCheckout1.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-e2e-vis-1"})
		recCheckout1 := httptest.NewRecorder()
		router.ServeHTTP(recCheckout1, reqCheckout1)

		if recCheckout1.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /checkout, got %d", recCheckout1.Code)
		}
		bodyCheckout1 := recCheckout1.Body.String()
		if strings.Contains(bodyCheckout1, `value="fake"`) {
			t.Errorf("expected /checkout to NOT contain fake payment option when disabled")
		}
		if !strings.Contains(bodyCheckout1, `value="card"`) {
			t.Errorf("expected /checkout to offer card payment")
		}
		if !strings.Contains(bodyCheckout1, `value="paypal"`) {
			t.Errorf("expected /checkout to offer paypal payment")
		}
		if !strings.Contains(bodyCheckout1, `value="cod"`) {
			t.Errorf("expected /checkout to offer cod payment")
		}

		// Step 3: Admin re-enables Fake provider, disables Stripe -> /checkout offers Fake, PayPal, Cash on Delivery (Card omitted).
		formEnableFake := url.Values{
			"enabled":     []string{"on"},
			"name":        []string{"Fake Payment Simulator"},
			"description": []string{"Interactive payment simulator for testing payment outcomes."},
		}
		reqEnableFake := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/fake", strings.NewReader(formEnableFake.Encode()))
		reqEnableFake.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setAdminSession(t, reqEnableFake)
		recEnableFake := httptest.NewRecorder()
		router.ServeHTTP(recEnableFake, reqEnableFake)

		if recEnableFake.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when re-enabling fake provider, got %d", recEnableFake.Code)
		}

		formDisableStripe := url.Values{
			"publishable_key": []string{"pk_test_sample"},
			"secret_key":      []string{"sk_test_sample"},
			// "enabled" omitted -> disabled
		}
		reqDisableStripe := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/stripe", strings.NewReader(formDisableStripe.Encode()))
		reqDisableStripe.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setAdminSession(t, reqDisableStripe)
		recDisableStripe := httptest.NewRecorder()
		router.ServeHTTP(recDisableStripe, reqDisableStripe)

		if recDisableStripe.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when disabling stripe provider, got %d", recDisableStripe.Code)
		}

		fakeCfg, _ = paySrv.FindProvider(ctx, paymentsDomain.ProviderFake)
		stripeCfg, _ := paySrv.FindProvider(ctx, paymentsDomain.ProviderStripe)
		if !fakeCfg.IsEnabled() {
			t.Errorf("expected Fake provider to be enabled")
		}
		if stripeCfg.IsEnabled() {
			t.Errorf("expected Stripe provider to be disabled")
		}

		// Verify /checkout with Stripe disabled and Fake enabled
		reqCheckout2 := httptest.NewRequest(http.MethodGet, "/checkout", nil)
		reqCheckout2.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-e2e-vis-1"})
		recCheckout2 := httptest.NewRecorder()
		router.ServeHTTP(recCheckout2, reqCheckout2)

		if recCheckout2.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /checkout, got %d", recCheckout2.Code)
		}
		bodyCheckout2 := recCheckout2.Body.String()
		if strings.Contains(bodyCheckout2, `value="card"`) {
			t.Errorf("expected /checkout to NOT offer card payment when Stripe is disabled")
		}
		if !strings.Contains(bodyCheckout2, `value="fake"`) {
			t.Errorf("expected /checkout to offer fake payment")
		}
		if !strings.Contains(bodyCheckout2, `value="paypal"`) {
			t.Errorf("expected /checkout to offer paypal payment")
		}
		if !strings.Contains(bodyCheckout2, `value="cod"`) {
			t.Errorf("expected /checkout to offer cod payment")
		}

		// Step 4: Re-enable both.
		formEnableStripe := url.Values{
			"enabled": []string{"on"},
		}
		reqEnableStripe := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/stripe", strings.NewReader(formEnableStripe.Encode()))
		reqEnableStripe.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setAdminSession(t, reqEnableStripe)
		recEnableStripe := httptest.NewRecorder()
		router.ServeHTTP(recEnableStripe, reqEnableStripe)

		if recEnableStripe.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when re-enabling stripe provider, got %d", recEnableStripe.Code)
		}

		fakeCfg, _ = paySrv.FindProvider(ctx, paymentsDomain.ProviderFake)
		stripeCfg, _ = paySrv.FindProvider(ctx, paymentsDomain.ProviderStripe)
		if !fakeCfg.IsEnabled() || !stripeCfg.IsEnabled() {
			t.Fatalf("expected both Fake and Stripe to be enabled, got fake=%v stripe=%v", fakeCfg.IsEnabled(), stripeCfg.IsEnabled())
		}

		// Verify /checkout offers all payment methods now
		reqCheckout3 := httptest.NewRequest(http.MethodGet, "/checkout", nil)
		reqCheckout3.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-e2e-vis-1"})
		recCheckout3 := httptest.NewRecorder()
		router.ServeHTTP(recCheckout3, reqCheckout3)

		if recCheckout3.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /checkout, got %d", recCheckout3.Code)
		}
		bodyCheckout3 := recCheckout3.Body.String()
		if !strings.Contains(bodyCheckout3, `value="card"`) {
			t.Errorf("expected /checkout to offer card payment")
		}
		if !strings.Contains(bodyCheckout3, `value="fake"`) {
			t.Errorf("expected /checkout to offer fake payment")
		}
		if !strings.Contains(bodyCheckout3, `value="paypal"`) {
			t.Errorf("expected /checkout to offer paypal payment")
		}
		if !strings.Contains(bodyCheckout3, `value="cod"`) {
			t.Errorf("expected /checkout to offer cod payment")
		}
	})

	t.Run("FakePayment_SuccessFlow", func(t *testing.T) {
		router, paySrv, checkoutSrv := setupE2EEnvironment(t)
		ctx := context.Background()

		// Step 1: Customer places order with Fake payment method.
		placeForm := url.Values{
			"payment_method": []string{"fake"},
			"ship_method":    []string{"flat"},
			"ship_name":      []string{"Jane Customer"},
			"ship_street1":   []string{"123 Maple Street"},
			"ship_city":      []string{"Portland"},
			"ship_zip":       []string{"97201"},
			"ship_country":   []string{"United States"},
		}
		reqPlace := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(placeForm.Encode()))
		reqPlace.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqPlace.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-e2e-success"})
		recPlace := httptest.NewRecorder()
		router.ServeHTTP(recPlace, reqPlace)

		if recPlace.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when placing order with fake payment, got %d: %s", recPlace.Code, recPlace.Body.String())
		}

		location := recPlace.Header().Get("Location")
		if !strings.HasPrefix(location, "/payments/fake/") {
			t.Fatalf("expected redirect to /payments/fake/{chargeID}, got: %s", location)
		}
		chargeID := strings.TrimPrefix(location, "/payments/fake/")
		if chargeID == "" {
			t.Fatalf("expected non-empty chargeID in redirect URL")
		}

		// Step 2: Verify charge is created in payments service with StatusPending and provider fake.
		charge, err := paySrv.FindCharge(ctx, chargeID)
		if err != nil {
			t.Fatalf("failed to find charge %s in payments service: %v", chargeID, err)
		}
		if charge.Status() != paymentsDomain.StatusPending {
			t.Errorf("expected charge status to be pending, got %s", charge.Status())
		}
		if charge.Provider() != paymentsDomain.ProviderFake {
			t.Errorf("expected charge provider to be fake, got %s", charge.Provider())
		}
		if charge.Amount() != 5500 { // 5000 cart subtotal + 500 flat shipping
			t.Errorf("expected charge amount 5500, got %d", charge.Amount())
		}
		if charge.Currency() != "USD" {
			t.Errorf("expected charge currency USD, got %s", charge.Currency())
		}

		orderID := charge.OrderID()
		if orderID == "" {
			t.Fatalf("expected charge to be linked to an orderID")
		}

		// Step 3: Verify order starts in StatusPending.
		orderView, err := checkoutSrv.Find(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to find order %s in checkout service: %v", orderID, err)
		}
		if orderView.Status() != checkoutDomain.StatusPending {
			t.Errorf("expected order status to start in pending, got %s", orderView.Status())
		}

		// Step 4: Customer lands on /payments/fake/{chargeID} simulator.
		reqSim := httptest.NewRequest(http.MethodGet, "/payments/fake/"+chargeID, nil)
		recSim := httptest.NewRecorder()
		router.ServeHTTP(recSim, reqSim)

		if recSim.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from fake payment simulator, got %d: %s", recSim.Code, recSim.Body.String())
		}
		simBody := recSim.Body.String()
		if !strings.Contains(simBody, "payment simulator") {
			t.Errorf("expected simulator page to have title containing 'payment simulator'")
		}
		if !strings.Contains(simBody, orderID) {
			t.Errorf("expected simulator page to display order ID %s", orderID)
		}
		if !strings.Contains(simBody, "Confirm Payment") {
			t.Errorf("expected simulator page to have 'Confirm Payment' button")
		}
		if !strings.Contains(simBody, "Reject Payment") {
			t.Errorf("expected simulator page to have 'Reject Payment' button")
		}

		// Step 5: Customer clicks 'Confirm Payment' -> calls /payments/fake/{chargeID}/confirm.
		reqConfirm := httptest.NewRequest(http.MethodPost, "/payments/fake/"+chargeID+"/confirm", nil)
		recConfirm := httptest.NewRecorder()
		router.ServeHTTP(recConfirm, reqConfirm)

		if recConfirm.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when confirming payment, got %d: %s", recConfirm.Code, recConfirm.Body.String())
		}
		expectedRedirect := "/order/" + orderID
		if recConfirm.Header().Get("Location") != expectedRedirect {
			t.Fatalf("expected redirect to %s, got: %s", expectedRedirect, recConfirm.Header().Get("Location"))
		}

		// Step 6: Verifies charge status becomes StatusSucceeded.
		updatedCharge, err := paySrv.FindCharge(ctx, chargeID)
		if err != nil {
			t.Fatalf("failed to re-fetch charge: %v", err)
		}
		if updatedCharge.Status() != paymentsDomain.StatusSucceeded {
			t.Errorf("expected charge status to be succeeded, got %s", updatedCharge.Status())
		}

		// Step 7: Verifies order status becomes StatusPaid.
		updatedOrder, err := checkoutSrv.Find(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to re-fetch order: %v", err)
		}
		if updatedOrder.Status() != checkoutDomain.StatusPaid {
			t.Errorf("expected order status to be paid, got %s", updatedOrder.Status())
		}

		// Step 8: Verifies redirection to /order/{orderID} with paid state rendered.
		reqOrder := httptest.NewRequest(http.MethodGet, "/order/"+orderID, nil)
		recOrder := httptest.NewRecorder()
		router.ServeHTTP(recOrder, reqOrder)

		if recOrder.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /order/%s, got %d", orderID, recOrder.Code)
		}
		orderBody := recOrder.Body.String()
		if !strings.Contains(orderBody, "thank you.") {
			t.Errorf("expected order confirmation page to have 'thank you.' header")
		}
		if !strings.Contains(orderBody, "order-status--paid") {
			t.Errorf("expected order confirmation page to have paid status badge")
		}
		if !strings.Contains(orderBody, orderID) {
			t.Errorf("expected order confirmation page to contain order ID %s", orderID)
		}

		// Extra idempotency check: revisiting simulator after payment succeeded redirects to /order/{orderID}
		reqSimRevisit := httptest.NewRequest(http.MethodGet, "/payments/fake/"+chargeID, nil)
		recSimRevisit := httptest.NewRecorder()
		router.ServeHTTP(recSimRevisit, reqSimRevisit)
		if recSimRevisit.Code != http.StatusSeeOther || recSimRevisit.Header().Get("Location") != expectedRedirect {
			t.Errorf("expected revisit of succeeded simulator to redirect to %s, got %d location: %s", expectedRedirect, recSimRevisit.Code, recSimRevisit.Header().Get("Location"))
		}
	})

	t.Run("FakePayment_RejectionFlow", func(t *testing.T) {
		router, paySrv, checkoutSrv := setupE2EEnvironment(t)
		ctx := context.Background()

		// Step 1: Customer places order with Fake payment method.
		placeForm := url.Values{
			"payment_method": []string{"fake"},
			"ship_method":    []string{"flat"},
			"ship_name":      []string{"Bob Customer"},
			"ship_street1":   []string{"456 Oak Avenue"},
			"ship_city":      []string{"Seattle"},
			"ship_zip":       []string{"98101"},
			"ship_country":   []string{"United States"},
		}
		reqPlace := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(placeForm.Encode()))
		reqPlace.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		reqPlace.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-e2e-reject"})
		recPlace := httptest.NewRecorder()
		router.ServeHTTP(recPlace, reqPlace)

		if recPlace.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when placing order with fake payment, got %d: %s", recPlace.Code, recPlace.Body.String())
		}

		location := recPlace.Header().Get("Location")
		if !strings.HasPrefix(location, "/payments/fake/") {
			t.Fatalf("expected redirect to /payments/fake/{chargeID}, got: %s", location)
		}
		chargeID := strings.TrimPrefix(location, "/payments/fake/")
		if chargeID == "" {
			t.Fatalf("expected non-empty chargeID in redirect URL")
		}

		// Step 2: Verifies charge created in payments service with StatusPending.
		charge, err := paySrv.FindCharge(ctx, chargeID)
		if err != nil {
			t.Fatalf("failed to find charge %s in payments service: %v", chargeID, err)
		}
		if charge.Status() != paymentsDomain.StatusPending {
			t.Errorf("expected charge status to be pending, got %s", charge.Status())
		}
		if charge.Provider() != paymentsDomain.ProviderFake {
			t.Errorf("expected charge provider to be fake, got %s", charge.Provider())
		}

		orderID := charge.OrderID()
		if orderID == "" {
			t.Fatalf("expected charge to be linked to an orderID")
		}

		// Step 3: Verifies order starts in StatusPending.
		orderView, err := checkoutSrv.Find(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to find order %s in checkout service: %v", orderID, err)
		}
		if orderView.Status() != checkoutDomain.StatusPending {
			t.Errorf("expected order status to start in pending, got %s", orderView.Status())
		}

		// Step 4: Customer lands on /payments/fake/{chargeID} simulator.
		reqSim := httptest.NewRequest(http.MethodGet, "/payments/fake/"+chargeID, nil)
		recSim := httptest.NewRecorder()
		router.ServeHTTP(recSim, reqSim)

		if recSim.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from fake payment simulator, got %d: %s", recSim.Code, recSim.Body.String())
		}
		simBody := recSim.Body.String()
		if !strings.Contains(simBody, orderID) {
			t.Errorf("expected simulator page to display order ID %s", orderID)
		}
		if !strings.Contains(simBody, "Confirm Payment") || !strings.Contains(simBody, "Reject Payment") {
			t.Errorf("expected simulator page to have confirm and reject buttons")
		}

		// Step 5: Customer selects reason 'insufficient_funds' and clicks 'Reject Payment' -> calls /payments/fake/{chargeID}/reject.
		rejectForm := url.Values{
			"reason": []string{"insufficient_funds"},
		}
		reqReject := httptest.NewRequest(http.MethodPost, "/payments/fake/"+chargeID+"/reject", strings.NewReader(rejectForm.Encode()))
		reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recReject := httptest.NewRecorder()
		router.ServeHTTP(recReject, reqReject)

		if recReject.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 See Other when rejecting payment, got %d: %s", recReject.Code, recReject.Body.String())
		}
		expectedRedirect := "/order/" + orderID
		if recReject.Header().Get("Location") != expectedRedirect {
			t.Fatalf("expected redirect to %s, got: %s", expectedRedirect, recReject.Header().Get("Location"))
		}

		// Step 6: Verifies charge status becomes StatusFailed with reason 'insufficient_funds'.
		updatedCharge, err := paySrv.FindCharge(ctx, chargeID)
		if err != nil {
			t.Fatalf("failed to re-fetch charge: %v", err)
		}
		if updatedCharge.Status() != paymentsDomain.StatusFailed {
			t.Errorf("expected charge status to be failed, got %s", updatedCharge.Status())
		}
		if updatedCharge.ProviderRef() != "insufficient_funds" {
			t.Errorf("expected charge reason to be 'insufficient_funds', got %s", updatedCharge.ProviderRef())
		}

		// Step 7: Verifies order status becomes StatusFailed.
		updatedOrder, err := checkoutSrv.Find(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to re-fetch order: %v", err)
		}
		if updatedOrder.Status() != checkoutDomain.StatusFailed {
			t.Errorf("expected order status to be failed, got %s", updatedOrder.Status())
		}
		if checkoutSrv.failedOrders[orderID] != "insufficient_funds" {
			t.Errorf("expected order failure reason 'insufficient_funds', got %s", checkoutSrv.failedOrders[orderID])
		}

		// Step 8: Verifies redirection to /order/{orderID} with 'payment declined.' title and return to cart/shopping links.
		reqOrder := httptest.NewRequest(http.MethodGet, "/order/"+orderID, nil)
		recOrder := httptest.NewRecorder()
		router.ServeHTTP(recOrder, reqOrder)

		if recOrder.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /order/%s, got %d: %s", orderID, recOrder.Code, recOrder.Body.String())
		}
		orderBody := recOrder.Body.String()
		if !strings.Contains(orderBody, "payment declined.") {
			t.Errorf("expected order page to have 'payment declined.' header")
		}
		if !strings.Contains(orderBody, "order-status--failed") {
			t.Errorf("expected order page to have failed status badge")
		}
		if !strings.Contains(orderBody, "Return to Cart") {
			t.Errorf("expected order page to have 'Return to Cart' link/button")
		}
		if !strings.Contains(orderBody, "Continue Shopping") {
			t.Errorf("expected order page to have 'Continue Shopping' link/button")
		}
		if !strings.Contains(orderBody, orderID) {
			t.Errorf("expected order page to contain order ID %s", orderID)
		}

		// Extra idempotency check: revisiting simulator after payment failed redirects to /order/{orderID}
		reqSimRevisit := httptest.NewRequest(http.MethodGet, "/payments/fake/"+chargeID, nil)
		recSimRevisit := httptest.NewRecorder()
		router.ServeHTTP(recSimRevisit, reqSimRevisit)
		if recSimRevisit.Code != http.StatusSeeOther || recSimRevisit.Header().Get("Location") != expectedRedirect {
			t.Errorf("expected revisit of failed simulator to redirect to %s, got %d location: %s", expectedRedirect, recSimRevisit.Code, recSimRevisit.Header().Get("Location"))
		}
	})
}
