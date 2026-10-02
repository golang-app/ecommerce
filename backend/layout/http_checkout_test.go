package layout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/ratelimit"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/sirupsen/logrus"
)

type mockCheckoutCommandsForCheckoutTest struct {
	methods     []checkoutDomain.ShippingMethod
	placeCalled bool
	placedOrder checkoutDomain.Order
	placeErr    error
}

func (m *mockCheckoutCommandsForCheckoutTest) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error) {
	m.placeCalled = true
	if m.placeErr != nil {
		return checkoutDomain.Order{}, m.placeErr
	}
	return m.placedOrder, nil
}

func (m *mockCheckoutCommandsForCheckoutTest) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}

func (m *mockCheckoutCommandsForCheckoutTest) AdminCancel(ctx context.Context, orderID string) error {
	return nil
}

func (m *mockCheckoutCommandsForCheckoutTest) MarkPaid(ctx context.Context, orderID string) error {
	return nil
}

func (m *mockCheckoutCommandsForCheckoutTest) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	return nil
}

func (m *mockCheckoutCommandsForCheckoutTest) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return m.methods, nil
}

func (m *mockCheckoutCommandsForCheckoutTest) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	for _, sm := range m.methods {
		if sm.Code() == code {
			return sm, nil
		}
	}
	return checkoutDomain.ShippingMethod{}, errors.New("shipping method not found")
}

func (m *mockCheckoutCommandsForCheckoutTest) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	return nil
}

func (m *mockCheckoutCommandsForCheckoutTest) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	return nil
}

type mockCheckoutCartService struct {
	cart *cartDomain.Cart
}

func (c *mockCheckoutCartService) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	return nil
}

func (c *mockCheckoutCartService) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if c.cart != nil {
		return c.cart, nil
	}
	return nil, cartDomain.ErrCartNotFound
}

func newTestCheckoutHandler(checkoutCmds checkoutCommands, cartSrv cartService) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)

	return httpHandler{
		cartSrv:     cartSrv,
		checkoutSrv: checkoutCmds,
		rates:       rates,
		logger:      logger,
	}
}

func TestCheckout_DisplaysOnlyEnabledShippingMethods(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
		checkoutDomain.NewShippingMethod("pickup", "Personal pickup", 0, false, "Store Pickup", true),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", false),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{methods: methods}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)

	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()

	handler.Checkout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Flat rate") {
		t.Errorf("expected body to contain 'Flat rate', but didn't")
	}
	if !strings.Contains(body, "Personal pickup") {
		t.Errorf("expected body to contain 'Personal pickup', but didn't")
	}
	if strings.Contains(body, "Express Courier") {
		t.Errorf("expected body NOT to contain disabled method 'Express Courier'")
	}
	if strings.Contains(body, `value="courier"`) {
		t.Errorf("expected body NOT to contain disabled method input with value='courier'")
	}
}

func TestPlaceOrder_RejectsDisabledShippingMethod(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", false),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{methods: methods}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)

	form := url.Values{
		"ship_method":    {"courier"},
		"payment_method": {"card"},
		"card_number":    {"4242424242424242"},
		"ship_name":      {"Jane Doe"},
		"ship_street1":   {"123 Main St"},
		"ship_city":      {"Portland"},
		"ship_zip":       {"97201"},
		"ship_country":   {"United States"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/checkout" {
		t.Fatalf("expected redirect to /checkout, got %s", loc)
	}
	if mockCmds.placeCalled {
		t.Fatalf("expected Place NOT to be called for disabled shipping method")
	}

	// Verify flash message
	reqWithCookies := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	for _, c := range rec.Result().Cookies() {
		reqWithCookies.AddCookie(c)
	}
	sess, _ := store.Get(reqWithCookies, "ecommerce")
	flashes := sess.Flashes("error")
	if len(flashes) == 0 {
		t.Fatalf("expected flash error message, got none")
	}
	expectedMsg := "The selected shipping method is currently unavailable"
	found := false
	for _, f := range flashes {
		if str, ok := f.(string); ok && str == expectedMsg {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected flash error %q, got %v", expectedMsg, flashes)
	}
}

func TestPlaceOrder_AcceptsEnabledShippingMethod(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", false),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{
		methods: methods,
		placedOrder: checkoutDomain.NewOrder(
			"order-123",
			"cart-123",
			"",
			checkoutDomain.Address{},
			methods[0],
			checkoutDomain.PaymentMethods()[0],
			nil,
			checkoutDomain.StatusPending,
			time.Now().UTC(),
		),
	}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)

	form := url.Values{
		"ship_method":    {"flat"},
		"payment_method": {"card"},
		"card_number":    {"4242424242424242"},
		"ship_name":      {"Jane Doe"},
		"ship_street1":   {"123 Main St"},
		"ship_city":      {"Portland"},
		"ship_zip":       {"97201"},
		"ship_country":   {"United States"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if !mockCmds.placeCalled {
		t.Fatalf("expected Place to be called for enabled shipping method")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/order/order-123" {
		t.Fatalf("expected redirect to /order/order-123, got %s", loc)
	}
}

func TestCheckout_ZeroShippingMethods_RendersUnavailableMessage(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", false),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", false),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{methods: methods}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)

	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()

	handler.Checkout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Shipping is currently unavailable") {
		t.Errorf("expected body to contain 'Shipping is currently unavailable', got:\n%s", body)
	}
	if !strings.Contains(body, "disabled") {
		t.Errorf("expected submit button to be disabled when shipping is unavailable, got:\n%s", body)
	}
}

func TestPlaceOrder_ZeroShippingMethods_RejectsOrder(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", false),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", false),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{methods: methods}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)

	form := url.Values{
		"ship_method":    {"flat"},
		"payment_method": {"card"},
		"card_number":    {"4242424242424242"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-123"})
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if mockCmds.placeCalled {
		t.Fatalf("expected Place NOT to be called when all shipping methods are disabled")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/checkout" {
		t.Fatalf("expected redirect to /checkout, got %s", loc)
	}

	s, _ := store.Get(req, "ecommerce")
	flashes := s.Flashes("error")
	if len(flashes) == 0 || !strings.Contains(flashes[0].(string), "Shipping is currently unavailable") {
		t.Errorf("expected 'Shipping is currently unavailable' error flash, got %v", flashes)
	}
}

func TestPlaceOrder_RateLimiting(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
	}
	mockCmds := &mockCheckoutCommandsForCheckoutTest{
		methods: methods,
		placedOrder: checkoutDomain.NewOrder(
			"order-rate-limit",
			"cart-rate-limit",
			"",
			checkoutDomain.Address{},
			methods[0],
			checkoutDomain.PaymentMethods()[0],
			nil,
			checkoutDomain.StatusPending,
			time.Now().UTC(),
		),
	}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCheckoutCartService{cart: cart}

	handler := newTestCheckoutHandler(mockCmds, cartSrv)
	handler.limiter = ratelimit.NewInMemory(ratelimit.DefaultRules())

	// Use a unique client IP to not interfere with other tests
	remoteIP := "198.51.100.99:50000"

	// 5 requests from this IP should succeed (burst capacity is 5)
	for i := 1; i <= 5; i++ {
		form := url.Values{
			"ship_method":    {"flat"},
			"payment_method": {"card"},
			"card_number":    {"4242424242424242"},
			"ship_name":      {"Jane Doe"},
			"ship_street1":   {"123 Main St"},
			"ship_city":      {"Portland"},
			"ship_zip":       {"97201"},
			"ship_country":   {"United States"},
		}
		req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = remoteIP
		req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-rate-limit"})
		rec := httptest.NewRecorder()

		handler.PlaceOrder(rec, req)

		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/order/order-rate-limit" {
			t.Fatalf("request %d: expected redirect to /order/order-rate-limit, got code %d, location %s", i, rec.Code, rec.Header().Get("Location"))
		}
	}

	// 6th request from the same IP must be rate-limited
	form := url.Values{
		"ship_method":    {"flat"},
		"payment_method": {"card"},
		"card_number":    {"4242424242424242"},
		"ship_name":      {"Jane Doe"},
		"ship_street1":   {"123 Main St"},
		"ship_city":      {"Portland"},
		"ship_zip":       {"97201"},
		"ship_country":   {"United States"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteIP
	req.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-rate-limit"})
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on rate limit, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/checkout" {
		t.Fatalf("expected redirect to /checkout on rate limit, got %s", loc)
	}

	// Check flash message
	reqWithCookies := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	for _, c := range rec.Result().Cookies() {
		reqWithCookies.AddCookie(c)
	}
	sess, _ := store.Get(reqWithCookies, "ecommerce")
	flashes := sess.Flashes("error")
	if len(flashes) == 0 {
		t.Fatalf("expected flash error message on rate limit, got none")
	}
	if flashes[0] != "Too many checkout attempts. Please try again in a moment." {
		t.Errorf("got flash %v, want 'Too many checkout attempts. Please try again in a moment.'", flashes[0])
	}
}
