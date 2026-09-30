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
	checkoutApp "github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	fulfillmentAdapter "github.com/bkielbasa/go-ecommerce/backend/fulfillment/adapter"
	fulfillmentApp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentDomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type shippingE2ECheckoutService struct {
	mu              sync.Mutex
	shippingStorage checkoutApp.ShippingStorage
	orders          map[string]*checkoutDomain.Order
	orderViews      map[string]checkoutQuery.OrderView
	nextID          int
}

func newShippingE2ECheckoutService() *shippingE2ECheckoutService {
	return &shippingE2ECheckoutService{
		shippingStorage: checkoutApp.NewInMemoryShippingStorage(),
		orders:          make(map[string]*checkoutDomain.Order),
		orderViews:      make(map[string]checkoutQuery.OrderView),
	}
}

func (s *shippingE2ECheckoutService) Place(
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
	orderID := fmt.Sprintf("ord-ship-e2e-%d", s.nextID)

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
		5000, 0, shipMethod.Cost(), 5000+shipMethod.Cost(),
		"USD", "", "", "", 0,
	)
	s.orderViews[orderID] = view

	return order, nil
}

func (s *shippingE2ECheckoutService) Cancel(ctx context.Context, orderID, customerID string) error {
	return s.AdminCancel(ctx, orderID)
}

func (s *shippingE2ECheckoutService) AdminCancel(ctx context.Context, orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if order, ok := s.orders[orderID]; ok {
		*order = checkoutDomain.NewOrder(
			order.ID(),
			order.UserID(),
			order.CustomerID(),
			order.ShipTo(),
			order.ShippingMethod(),
			order.PaymentMethod(),
			order.Items(),
			checkoutDomain.StatusCancelled,
			order.PlacedAt(),
		)
	}
	if view, ok := s.orderViews[orderID]; ok {
		s.orderViews[orderID] = checkoutQuery.NewOrderView(
			view.ID(),
			view.CustomerID(),
			checkoutDomain.StatusCancelled,
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

func (s *shippingE2ECheckoutService) MarkPaid(ctx context.Context, orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func (s *shippingE2ECheckoutService) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func (s *shippingE2ECheckoutService) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return s.shippingStorage.ListShippingMethods(ctx)
}

func (s *shippingE2ECheckoutService) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	return s.shippingStorage.FindShippingMethod(ctx, code)
}

func (s *shippingE2ECheckoutService) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, err := s.shippingStorage.FindShippingMethod(ctx, code)
	if err != nil {
		return err
	}
	updated := checkoutDomain.NewShippingMethod(code, label, cost, existing.RequiresAddress(), carrier, enabled)
	return s.shippingStorage.SaveShippingMethod(ctx, updated)
}

func (s *shippingE2ECheckoutService) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if view, ok := s.orderViews[orderID]; ok {
		s.orderViews[orderID] = checkoutQuery.NewOrderView(
			view.ID(),
			view.CustomerID(),
			view.Status(),
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
			carrier,
			trackingCode,
			view.DiscountCode(),
			view.DiscountAmount(),
		)
	}
	return nil
}

func (s *shippingE2ECheckoutService) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if view, ok := s.orderViews[id]; ok {
		return view, nil
	}
	return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
}

func (s *shippingE2ECheckoutService) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (s *shippingE2ECheckoutService) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (s *shippingE2ECheckoutService) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}

func (s *shippingE2ECheckoutService) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}

func (s *shippingE2ECheckoutService) seedOrder(
	orderID string,
	status checkoutDomain.Status,
	carrier, trackingCode string,
	shipMethod checkoutDomain.ShippingMethod,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := []checkoutDomain.Line{
		checkoutDomain.NewLine("prod-1", "Test Product", 1, 5000, "USD"),
	}
	shipTo, _ := checkoutDomain.NewAddress("John Doe", "123 Elm St", "", "Springfield", "12345", "US")
	payMethod, _ := checkoutDomain.PaymentMethodByCode("cod")

	order := checkoutDomain.NewOrder(
		orderID,
		"sess-seed",
		"cust-seed",
		shipTo,
		shipMethod,
		payMethod,
		items,
		status,
		time.Now().UTC(),
	)
	s.orders[orderID] = &order

	view := checkoutQuery.NewOrderView(
		orderID,
		"cust-seed",
		status,
		time.Now().UTC(),
		items,
		shipTo,
		shipMethod,
		payMethod,
		5000, 0, shipMethod.Cost(), 5000+shipMethod.Cost(),
		"USD", carrier, trackingCode, "", 0,
	)
	s.orderViews[orderID] = view
}

type shippingE2ECartService struct {
	cart *cartDomain.Cart
}

func (c *shippingE2ECartService) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	return nil
}

func (c *shippingE2ECartService) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if c.cart != nil {
		return c.cart, nil
	}
	return nil, cartDomain.ErrCartNotFound
}

func setupShippingE2EEnvironment(t *testing.T) (*mux.Router, *shippingE2ECheckoutService, *fulfillmentApp.Service) {
	t.Helper()
	setupTestEnvironment(t)

	checkoutSrv := newShippingE2ECheckoutService()

	fulfillmentStorage := fulfillmentAdapter.NewInMemory()
	fulfillmentSrv := fulfillmentApp.NewService(fulfillmentStorage)

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &shippingE2ECartService{cart: cart}

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
		fulfillmentSrv,
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
		nil, // paymentsSrv
	)

	router := mux.NewRouter()
	bc.(*boundedContext).MuxRegister(router)
	return router, checkoutSrv, fulfillmentSrv
}

func TestShippingProviders_EndToEnd(t *testing.T) {
	router, checkoutSrv, _ := setupShippingE2EEnvironment(t)
	ctx := context.Background()

	// Step 1: Admin visits GET /admin/shipping-providers and verifies default methods.
	reqList := httptest.NewRequest(http.MethodGet, "/admin/shipping-providers", nil)
	setAdminSession(t, reqList)
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /admin/shipping-providers, got %d", recList.Code)
	}
	bodyList := recList.Body.String()
	if !strings.Contains(bodyList, "Flat rate") {
		t.Errorf("expected admin shipping providers page to display 'Flat rate'")
	}
	if !strings.Contains(bodyList, "Personal pickup") {
		t.Errorf("expected admin shipping providers page to display 'Personal pickup'")
	}
	if !strings.Contains(bodyList, "Courier") {
		t.Errorf("expected admin shipping providers page to display 'Courier'")
	}

	// Step 2: Admin updates shipping provider configuration:
	// - Updates flat: enabled=1, label=FedEx Express Delivery, cost=12.50, carrier=FedEx
	// - Disables courier: enabled=0
	formFlat := url.Values{
		"enabled": []string{"1"},
		"label":   []string{"FedEx Express Delivery"},
		"cost":    []string{"12.50"},
		"carrier": []string{"FedEx"},
	}
	reqUpdateFlat := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/flat", strings.NewReader(formFlat.Encode()))
	reqUpdateFlat.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqUpdateFlat)
	recUpdateFlat := httptest.NewRecorder()
	router.ServeHTTP(recUpdateFlat, reqUpdateFlat)

	if recUpdateFlat.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when updating flat shipping provider, got %d", recUpdateFlat.Code)
	}
	if loc := recUpdateFlat.Header().Get("Location"); loc != "/admin/shipping-providers" {
		t.Fatalf("expected redirect to /admin/shipping-providers, got %s", loc)
	}

	// Disable courier (omitting enabled checkbox or enabled=0)
	formCourier := url.Values{
		"enabled": []string{"0"},
		"label":   []string{"Courier"},
		"cost":    []string{"15.00"},
		"carrier": []string{"Express Courier"},
	}
	reqDisableCourier := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/courier", strings.NewReader(formCourier.Encode()))
	reqDisableCourier.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqDisableCourier)
	recDisableCourier := httptest.NewRecorder()
	router.ServeHTTP(recDisableCourier, reqDisableCourier)

	if recDisableCourier.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when disabling courier shipping provider, got %d", recDisableCourier.Code)
	}
	if loc := recDisableCourier.Header().Get("Location"); loc != "/admin/shipping-providers" {
		t.Fatalf("expected redirect to /admin/shipping-providers, got %s", loc)
	}

	// Verify persistence in shipping storage
	flatMethod, err := checkoutSrv.FindShippingMethod(ctx, "flat")
	if err != nil {
		t.Fatalf("failed to find flat method in storage: %v", err)
	}
	if !flatMethod.IsEnabled() {
		t.Errorf("expected flat method to be enabled")
	}
	if flatMethod.Label() != "FedEx Express Delivery" {
		t.Errorf("expected flat method label 'FedEx Express Delivery', got %q", flatMethod.Label())
	}
	if flatMethod.Cost() != 1250 {
		t.Errorf("expected flat method cost 1250, got %d", flatMethod.Cost())
	}
	if flatMethod.Carrier() != "FedEx" {
		t.Errorf("expected flat method carrier 'FedEx', got %q", flatMethod.Carrier())
	}

	courierMethod, err := checkoutSrv.FindShippingMethod(ctx, "courier")
	if err != nil {
		t.Fatalf("failed to find courier method in storage: %v", err)
	}
	if courierMethod.IsEnabled() {
		t.Errorf("expected courier method to be disabled")
	}

	// Step 3: Customer with cart visits GET /checkout.
	// Verifies FedEx Express Delivery is present with 12.50, and courier is NOT offered.
	reqCheckout := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	reqCheckout.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-shipping-e2e"})
	recCheckout := httptest.NewRecorder()
	router.ServeHTTP(recCheckout, reqCheckout)

	if recCheckout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /checkout, got %d", recCheckout.Code)
	}
	bodyCheckout := recCheckout.Body.String()
	if !strings.Contains(bodyCheckout, "FedEx Express Delivery") {
		t.Errorf("expected /checkout to display 'FedEx Express Delivery'")
	}
	if !strings.Contains(bodyCheckout, "12.50") {
		t.Errorf("expected /checkout to display '12.50'")
	}
	if strings.Contains(bodyCheckout, `value="courier"`) {
		t.Errorf("expected /checkout to NOT offer disabled courier method")
	}

	// Step 4: Placing order with disabled method (ship_method=courier) is rejected.
	formReject := url.Values{
		"ship_method":    {"courier"},
		"payment_method": {"cod"},
		"ship_name":      {"John Customer"},
		"ship_street1":   {"100 Pine Street"},
		"ship_city":      {"Seattle"},
		"ship_zip":       {"98101"},
		"ship_country":   {"United States"},
	}
	reqReject := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(formReject.Encode()))
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReject.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-shipping-e2e"})
	recReject := httptest.NewRecorder()
	router.ServeHTTP(recReject, reqReject)

	if recReject.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other on placing order with disabled shipping method, got %d", recReject.Code)
	}
	if loc := recReject.Header().Get("Location"); loc != "/checkout" {
		t.Fatalf("expected redirect to /checkout, got %s", loc)
	}

	// Verify flash error from session cookie
	cookieReq := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	for _, c := range recReject.Result().Cookies() {
		cookieReq.AddCookie(c)
	}
	rejectSession, _ := store.Get(cookieReq, "ecommerce")
	errorFlashes := rejectSession.Flashes("error")
	var foundUnavailableFlash bool
	for _, f := range errorFlashes {
		if str, ok := f.(string); ok && strings.Contains(str, "The selected shipping method is currently unavailable") {
			foundUnavailableFlash = true
			break
		}
	}
	if !foundUnavailableFlash {
		t.Errorf("expected flash error 'The selected shipping method is currently unavailable', got flashes: %v", errorFlashes)
	}

	// Step 5: Placing order with enabled method (ship_method=flat) succeeds.
	formSuccess := url.Values{
		"ship_method":    {"flat"},
		"payment_method": {"cod"},
		"ship_name":      {"John Customer"},
		"ship_street1":   {"100 Pine Street"},
		"ship_city":      {"Seattle"},
		"ship_zip":       {"98101"},
		"ship_country":   {"United States"},
	}
	reqSuccess := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(formSuccess.Encode()))
	reqSuccess.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSuccess.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-shipping-e2e"})
	recSuccess := httptest.NewRecorder()
	router.ServeHTTP(recSuccess, reqSuccess)

	if recSuccess.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other on valid order placement, got %d", recSuccess.Code)
	}
	successLoc := recSuccess.Header().Get("Location")
	if !strings.HasPrefix(successLoc, "/order/") {
		t.Fatalf("expected redirect to /order/{orderID}, got %s", successLoc)
	}
	createdOrderID := strings.TrimPrefix(successLoc, "/order/")
	if createdOrderID == "" {
		t.Fatalf("expected non-empty order ID in redirect location")
	}

	// Verify order state
	orderView, err := checkoutSrv.Find(ctx, createdOrderID)
	if err != nil {
		t.Fatalf("failed to find created order %s in checkout service: %v", createdOrderID, err)
	}
	if orderView.Status() != checkoutDomain.StatusPending {
		t.Errorf("expected order status pending, got %s", orderView.Status())
	}
	if orderView.Carrier() != "" {
		t.Errorf("expected initial carrier to be empty, got %q", orderView.Carrier())
	}
	if orderView.TrackingCode() != "" {
		t.Errorf("expected initial tracking code to be empty, got %q", orderView.TrackingCode())
	}
	if orderView.ShippingMethod().Code() != "flat" {
		t.Errorf("expected shipping method code 'flat', got %q", orderView.ShippingMethod().Code())
	}
	if orderView.ShippingMethod().Label() != "FedEx Express Delivery" {
		t.Errorf("expected shipping method label 'FedEx Express Delivery', got %q", orderView.ShippingMethod().Label())
	}
	if orderView.ShippingMethod().Cost() != 1250 {
		t.Errorf("expected shipping method cost 1250, got %d", orderView.ShippingMethod().Cost())
	}
}

func TestAdminOrderStatusAndTracking_EndToEnd(t *testing.T) {
	router, checkoutSrv, fulfillmentSrv := setupShippingE2EEnvironment(t)
	ctx := context.Background()

	// Seed a pending order with empty carrier and tracking
	orderID := "ord-status-e2e-1"
	flatMethod, _ := checkoutSrv.FindShippingMethod(ctx, "flat")
	checkoutSrv.seedOrder(orderID, checkoutDomain.StatusPending, "", "", flatMethod)

	// Step 1: Admin updates carrier and tracking number on the pending order.
	formStep1 := url.Values{
		"payment_status":  {"pending"},
		"delivery_status": {""},
		"carrier":         {"DHL Express"},
		"tracking_code":   {"DHL-987654"},
	}
	reqStep1 := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(formStep1.Encode()))
	reqStep1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqStep1)
	recStep1 := httptest.NewRecorder()
	router.ServeHTTP(recStep1, reqStep1)

	if recStep1.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when updating carrier and tracking, got %d (body: %s)", recStep1.Code, recStep1.Body.String())
	}
	if loc := recStep1.Header().Get("Location"); loc != "/admin/orders/"+orderID {
		t.Fatalf("expected redirect to /admin/orders/%s, got %s", orderID, loc)
	}

	// Verify order view reflects updated carrier and tracking
	viewStep1, err := checkoutSrv.Find(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to find order %s: %v", orderID, err)
	}
	if viewStep1.Carrier() != "DHL Express" {
		t.Errorf("expected order carrier 'DHL Express', got %q", viewStep1.Carrier())
	}
	if viewStep1.TrackingCode() != "DHL-987654" {
		t.Errorf("expected order tracking code 'DHL-987654', got %q", viewStep1.TrackingCode())
	}
	if viewStep1.Status() != checkoutDomain.StatusPending {
		t.Errorf("expected order status pending, got %s", viewStep1.Status())
	}

	// Verify admin order detail page renders carrier and tracking code
	reqDetail := httptest.NewRequest(http.MethodGet, "/admin/orders/"+orderID, nil)
	setAdminSession(t, reqDetail)
	recDetail := httptest.NewRecorder()
	router.ServeHTTP(recDetail, reqDetail)

	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /admin/orders/%s, got %d", orderID, recDetail.Code)
	}
	bodyDetail := recDetail.Body.String()
	if !strings.Contains(bodyDetail, "DHL Express") {
		t.Errorf("expected order detail page to contain 'DHL Express'")
	}
	if !strings.Contains(bodyDetail, "DHL-987654") {
		t.Errorf("expected order detail page to contain 'DHL-987654'")
	}

	// Step 2: Admin changes payment status to paid.
	// Fulfillment aggregate is created and synchronized with carrier and tracking number.
	formStep2 := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"scheduled"},
		"carrier":         {"DHL Express"},
		"tracking_code":   {"DHL-987654"},
	}
	reqStep2 := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(formStep2.Encode()))
	reqStep2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqStep2)
	recStep2 := httptest.NewRecorder()
	router.ServeHTTP(recStep2, reqStep2)

	if recStep2.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when updating payment status to paid, got %d", recStep2.Code)
	}

	viewStep2, err := checkoutSrv.Find(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to find order %s: %v", orderID, err)
	}
	if viewStep2.Status() != checkoutDomain.StatusPaid {
		t.Errorf("expected order status paid, got %s", viewStep2.Status())
	}

	ffStep2, err := fulfillmentSrv.ByOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("expected fulfillment aggregate to exist for order %s: %v", orderID, err)
	}
	if ffStep2.Status() != fulfillmentDomain.StatusScheduled {
		t.Errorf("expected fulfillment status scheduled, got %s", ffStep2.Status())
	}
	if ffStep2.Carrier() != "DHL Express" {
		t.Errorf("expected fulfillment carrier 'DHL Express', got %q", ffStep2.Carrier())
	}
	if ffStep2.TrackingCode() != "DHL-987654" {
		t.Errorf("expected fulfillment tracking code 'DHL-987654', got %q", ffStep2.TrackingCode())
	}

	// Step 3: Admin changes delivery status progression: shipped -> delivered.
	formShipped := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"shipped"},
		"carrier":         {"DHL Express"},
		"tracking_code":   {"DHL-987654"},
	}
	reqShipped := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(formShipped.Encode()))
	reqShipped.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqShipped)
	recShipped := httptest.NewRecorder()
	router.ServeHTTP(recShipped, reqShipped)

	if recShipped.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when setting delivery status to shipped, got %d", recShipped.Code)
	}

	ffShipped, err := fulfillmentSrv.ByOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to load fulfillment after shipping: %v", err)
	}
	if ffShipped.Status() != fulfillmentDomain.StatusShipped {
		t.Errorf("expected fulfillment status shipped, got %s", ffShipped.Status())
	}
	if ffShipped.Carrier() != "DHL Express" {
		t.Errorf("expected fulfillment carrier 'DHL Express', got %q", ffShipped.Carrier())
	}
	if ffShipped.TrackingCode() != "DHL-987654" {
		t.Errorf("expected fulfillment tracking code 'DHL-987654', got %q", ffShipped.TrackingCode())
	}

	formDelivered := url.Values{
		"payment_status":  {"paid"},
		"delivery_status": {"delivered"},
		"carrier":         {"DHL Express"},
		"tracking_code":   {"DHL-987654"},
	}
	reqDelivered := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID+"/status", strings.NewReader(formDelivered.Encode()))
	reqDelivered.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqDelivered)
	recDelivered := httptest.NewRecorder()
	router.ServeHTTP(recDelivered, reqDelivered)

	if recDelivered.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when setting delivery status to delivered, got %d", recDelivered.Code)
	}

	ffDelivered, err := fulfillmentSrv.ByOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to load fulfillment after delivery: %v", err)
	}
	if ffDelivered.Status() != fulfillmentDomain.StatusDelivered {
		t.Errorf("expected fulfillment status delivered, got %s", ffDelivered.Status())
	}
	if ffDelivered.Carrier() != "DHL Express" {
		t.Errorf("expected fulfillment carrier 'DHL Express', got %q", ffDelivered.Carrier())
	}
	if ffDelivered.TrackingCode() != "DHL-987654" {
		t.Errorf("expected fulfillment tracking code 'DHL-987654', got %q", ffDelivered.TrackingCode())
	}

	// Step 4: Admin marks payment status cancelled and failed.
	// Seed second pending order and cancel it
	orderID2 := "ord-status-e2e-2"
	checkoutSrv.seedOrder(orderID2, checkoutDomain.StatusPending, "", "", flatMethod)

	formCancel := url.Values{
		"payment_status": {"cancelled"},
	}
	reqCancel := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID2+"/status", strings.NewReader(formCancel.Encode()))
	reqCancel.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqCancel)
	recCancel := httptest.NewRecorder()
	router.ServeHTTP(recCancel, reqCancel)

	if recCancel.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when cancelling order, got %d", recCancel.Code)
	}

	viewCancel, err := checkoutSrv.Find(ctx, orderID2)
	if err != nil {
		t.Fatalf("failed to find order %s: %v", orderID2, err)
	}
	if viewCancel.Status() != checkoutDomain.StatusCancelled {
		t.Errorf("expected order status cancelled, got %s", viewCancel.Status())
	}

	// Seed third pending order and fail it
	orderID3 := "ord-status-e2e-3"
	checkoutSrv.seedOrder(orderID3, checkoutDomain.StatusPending, "", "", flatMethod)

	formFail := url.Values{
		"payment_status": {"failed"},
	}
	reqFail := httptest.NewRequest(http.MethodPost, "/admin/orders/"+orderID3+"/status", strings.NewReader(formFail.Encode()))
	reqFail.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqFail)
	recFail := httptest.NewRecorder()
	router.ServeHTTP(recFail, reqFail)

	if recFail.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other when failing order, got %d", recFail.Code)
	}

	viewFail, err := checkoutSrv.Find(ctx, orderID3)
	if err != nil {
		t.Fatalf("failed to find order %s: %v", orderID3, err)
	}
	if viewFail.Status() != checkoutDomain.StatusFailed {
		t.Errorf("expected order status failed, got %s", viewFail.Status())
	}
}
