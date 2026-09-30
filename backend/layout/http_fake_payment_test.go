package layout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type mockCheckoutCmds struct {
	paidOrders   []string
	failedOrders map[string]string // orderID -> reason
	placeFn      func() (checkoutDomain.Order, error)
}

func (m *mockCheckoutCmds) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error) {
	if m.placeFn != nil {
		return m.placeFn()
	}
	payM, _ := checkoutDomain.PaymentMethodByCode(payMethod.Code())
	order := checkoutDomain.NewOrder(
		"ord-fake-1",
		sessID,
		customerID,
		shipTo,
		shipMethod,
		payM,
		[]checkoutDomain.Line{
			checkoutDomain.NewLine("prod-1", "Product 1", 1, 5000, "USD"),
		},
		checkoutDomain.StatusPending,
		time.Now(),
	)
	return order, nil
}

func (m *mockCheckoutCmds) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}

func (m *mockCheckoutCmds) AdminCancel(ctx context.Context, orderID string) error {
	return nil
}

func (m *mockCheckoutCmds) MarkPaid(ctx context.Context, orderID string) error {
	m.paidOrders = append(m.paidOrders, orderID)
	return nil
}

func (m *mockCheckoutCmds) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	if m.failedOrders == nil {
		m.failedOrders = make(map[string]string)
	}
	m.failedOrders[orderID] = reason
	return nil
}

type mockCheckoutQrys struct {
	orders map[string]checkoutQuery.OrderView
}

func (m *mockCheckoutQrys) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	if o, ok := m.orders[id]; ok {
		return o, nil
	}
	return checkoutQuery.OrderView{}, checkoutDomain.ErrOrderNotFound
}

func (m *mockCheckoutQrys) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (m *mockCheckoutQrys) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}

func (m *mockCheckoutQrys) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}

func (m *mockCheckoutQrys) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}

type mockCartSrv struct {
	cart *cartDomain.Cart
}

func (m *mockCartSrv) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	return nil
}
func (m *mockCartSrv) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if m.cart != nil {
		return m.cart, nil
	}
	return nil, cartDomain.ErrCartNotFound
}

func setupFakePaymentEnv(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err == nil {
		if filepath.Base(origWd) == "layout" {
			if err := os.Chdir(".."); err != nil {
				t.Fatalf("failed to chdir to parent: %v", err)
			}
			t.Cleanup(func() {
				_ = os.Chdir(origWd)
			})
		}
	}
	store = newCookieStore([]byte("test-secret-12345678901234567890"), false)
	setCSRFEnabled(false)
}

func newFakePaymentTestHandler(paymentsSrv paymentsService, checkoutCmds checkoutCommands, checkoutQrys checkoutQueries, cartSrv cartService) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)

	return httpHandler{
		paymentsSrv: paymentsSrv,
		checkoutSrv: checkoutCmds,
		checkoutQry: checkoutQrys,
		cartSrv:     cartSrv,
		rates:       rates,
		logger:      logger,
	}
}

func TestFakePaymentSimulator_ViewPending(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-123", 5000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	orderView := checkoutQuery.NewOrderView(
		"ord-123", "cust-1", checkoutDomain.StatusPending, time.Now(),
		[]checkoutDomain.Line{
			checkoutDomain.NewLine("prod-1", "Go T-Shirt", 2, 2500, "USD"),
		},
		checkoutDomain.Address{},
		checkoutDomain.ShippingMethod{},
		checkoutDomain.PaymentMethod{},
		5000, 0, 0, 5000, "USD", "", "", "", 0,
	)

	qrys := &mockCheckoutQrys{orders: map[string]checkoutQuery.OrderView{"ord-123": orderView}}
	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, qrys, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/fake/"+charge.ID(), nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentSimulator(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "payment simulator") {
		t.Errorf("body missing 'payment simulator'")
	}
	if !strings.Contains(body, "Test Payment Gateway Simulator") {
		t.Errorf("body missing 'Test Payment Gateway Simulator'")
	}
	if !strings.Contains(body, "ord-123") {
		t.Errorf("body missing order ID 'ord-123'")
	}
	if !strings.Contains(body, "Confirm Payment") {
		t.Errorf("body missing 'Confirm Payment'")
	}
	if !strings.Contains(body, "Reject Payment") {
		t.Errorf("body missing 'Reject Payment'")
	}
	if !strings.Contains(body, "insufficient_funds") {
		t.Errorf("body missing decline reason option 'insufficient_funds'")
	}
	if !strings.Contains(body, "card_declined") {
		t.Errorf("body missing decline reason option 'card_declined'")
	}
}

func TestFakePaymentSimulator_RedirectIfSucceeded(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-123", 5000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}
	_, err = paySrv.ConfirmCharge(ctx, charge.ID())
	if err != nil {
		t.Fatalf("ConfirmCharge: %v", err)
	}

	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, &mockCheckoutQrys{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/fake/"+charge.ID(), nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentSimulator(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/order/ord-123" {
		t.Errorf("expected redirect to /order/ord-123, got %s", loc)
	}
}

func TestFakePaymentSimulator_RedirectIfFailed(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-123", 5000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}
	_, err = paySrv.RejectCharge(ctx, charge.ID(), "insufficient_funds")
	if err != nil {
		t.Fatalf("RejectCharge: %v", err)
	}

	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, &mockCheckoutQrys{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/fake/"+charge.ID(), nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentSimulator(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/order/ord-123" {
		t.Errorf("expected redirect to /order/ord-123, got %s", loc)
	}
}

func TestFakePaymentSimulator_NotFound(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, &mockCheckoutQrys{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/payments/fake/nonexistent", nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": "nonexistent"})
	rec := httptest.NewRecorder()

	handler.FakePaymentSimulator(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestFakePaymentConfirm(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-confirm-1", 5000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, &mockCheckoutQrys{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/payments/fake/"+charge.ID()+"/confirm", nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentConfirm(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/order/ord-confirm-1" {
		t.Errorf("expected redirect to /order/ord-confirm-1, got %s", loc)
	}

	// Verify charge status transitioned to succeeded
	updatedCharge, err := paySrv.FindCharge(ctx, charge.ID())
	if err != nil {
		t.Fatalf("FindCharge: %v", err)
	}
	if updatedCharge.Status() != paymentsDomain.StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got %s", updatedCharge.Status())
	}

	// Verify MarkPaid was called
	if len(cmds.paidOrders) != 1 || cmds.paidOrders[0] != "ord-confirm-1" {
		t.Errorf("expected MarkPaid called for ord-confirm-1, got %v", cmds.paidOrders)
	}
}

func TestFakePaymentReject(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-reject-1", 5000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, &mockCheckoutQrys{}, nil)

	form := url.Values{"reason": []string{"suspected_fraud"}}
	req := httptest.NewRequest(http.MethodPost, "/payments/fake/"+charge.ID()+"/reject", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentReject(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/order/ord-reject-1" {
		t.Errorf("expected redirect to /order/ord-reject-1, got %s", loc)
	}

	// Verify charge status transitioned to failed
	updatedCharge, err := paySrv.FindCharge(ctx, charge.ID())
	if err != nil {
		t.Fatalf("FindCharge: %v", err)
	}
	if updatedCharge.Status() != paymentsDomain.StatusFailed {
		t.Errorf("expected StatusFailed, got %s", updatedCharge.Status())
	}

	// Verify MarkPaymentFailed was called with reason
	if cmds.failedOrders["ord-reject-1"] != "suspected_fraud" {
		t.Errorf("expected reason suspected_fraud, got %q", cmds.failedOrders["ord-reject-1"])
	}
}

func TestFakePaymentReject_DefaultReason(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	charge, err := paySrv.CreatePendingCharge(ctx, "ord-reject-2", 3000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, &mockCheckoutQrys{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/payments/fake/"+charge.ID()+"/reject", nil)
	req = mux.SetURLVars(req, map[string]string{"chargeID": charge.ID()})
	rec := httptest.NewRecorder()

	handler.FakePaymentReject(rec, req)

	if cmds.failedOrders["ord-reject-2"] != "insufficient_funds" {
		t.Errorf("expected default reason insufficient_funds, got %q", cmds.failedOrders["ord-reject-2"])
	}
}

func TestCheckout_PaymentMethodFiltering(t *testing.T) {
	ctx := context.Background()

	// 1. Both stripe and fake enabled
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, &mockCheckoutQrys{}, nil)

	methods := handler.availablePaymentMethods(ctx)
	codes := make(map[string]bool)
	for _, m := range methods {
		codes[m.Code()] = true
	}
	if !codes["card"] || !codes["fake"] || !codes["paypal"] || !codes["cod"] {
		t.Errorf("expected all methods enabled, got %v", codes)
	}

	// 2. Disable Stripe -> "card" should be omitted
	if err := paySrv.UpdateProvider(ctx, paymentsDomain.ProviderStripe, false, nil); err != nil {
		t.Fatalf("UpdateProvider stripe: %v", err)
	}
	methods = handler.availablePaymentMethods(ctx)
	codes = make(map[string]bool)
	for _, m := range methods {
		codes[m.Code()] = true
	}
	if codes["card"] {
		t.Errorf("expected 'card' to be omitted when stripe is disabled")
	}
	if !codes["fake"] || !codes["paypal"] || !codes["cod"] {
		t.Errorf("expected fake, paypal, cod to remain enabled")
	}

	// 3. Disable Fake, re-enable Stripe -> "fake" omitted, "card" present
	if err := paySrv.UpdateProvider(ctx, paymentsDomain.ProviderStripe, true, nil); err != nil {
		t.Fatalf("UpdateProvider stripe: %v", err)
	}
	if err := paySrv.UpdateProvider(ctx, paymentsDomain.ProviderFake, false, nil); err != nil {
		t.Fatalf("UpdateProvider fake: %v", err)
	}
	methods = handler.availablePaymentMethods(ctx)
	codes = make(map[string]bool)
	for _, m := range methods {
		codes[m.Code()] = true
	}
	if codes["fake"] {
		t.Errorf("expected 'fake' to be omitted when fake is disabled")
	}
	if !codes["card"] {
		t.Errorf("expected 'card' to be present when stripe is enabled")
	}
}

func TestCheckout_PlaceOrder_DisabledPaymentMethodRejected(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	// Disable fake payment provider
	if err := paySrv.UpdateProvider(ctx, paymentsDomain.ProviderFake, false, nil); err != nil {
		t.Fatalf("UpdateProvider fake: %v", err)
	}

	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, &mockCheckoutQrys{}, nil)

	form := url.Values{
		"payment_method": []string{"fake"},
		"ship_method":    []string{"flat"},
		"ship_name":      []string{"John Doe"},
		"ship_street1":   []string{"123 Elm St"},
		"ship_city":      []string{"Springfield"},
		"ship_zip":       []string{"12345"},
		"ship_country":   []string{"USA"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/checkout" {
		t.Errorf("expected redirect to /checkout, got %s", loc)
	}

	// Read session flash to verify error message
	session, _ := store.Get(req, "ecommerce")
	flashes := session.Flashes("error")
	foundMsg := false
	for _, f := range flashes {
		if str, ok := f.(string); ok && strings.Contains(str, "selected payment method is currently unavailable") {
			foundMsg = true
			break
		}
	}
	if !foundMsg {
		t.Errorf("expected flash error 'selected payment method is currently unavailable', got %v", flashes)
	}
}

func TestCheckout_PlaceOrder_FakePaymentRedirectsToSimulator(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, func() string { return "ch-test-fake" }, nil)

	cmds := &mockCheckoutCmds{}
	handler := newFakePaymentTestHandler(paySrv, cmds, &mockCheckoutQrys{}, nil)

	form := url.Values{
		"payment_method": []string{"fake"},
		"ship_method":    []string{"flat"},
		"ship_name":      []string{"John Doe"},
		"ship_street1":   []string{"123 Elm St"},
		"ship_city":      []string{"Springfield"},
		"ship_zip":       []string{"12345"},
		"ship_country":   []string{"USA"},
	}
	req := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.PlaceOrder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/payments/fake/ch-test-fake" {
		t.Errorf("expected redirect to /payments/fake/ch-test-fake, got %s", loc)
	}
	if rec.Header().Get("HX-Trigger") != "cartBudge" {
		t.Errorf("expected HX-Trigger header cartBudge, got %q", rec.Header().Get("HX-Trigger"))
	}

	// Verify pending charge was created with provider "fake"
	ctx := context.Background()
	charge, err := paySrv.FindCharge(ctx, "ch-test-fake")
	if err != nil {
		t.Fatalf("FindCharge: %v", err)
	}
	if charge.Provider() != paymentsDomain.ProviderFake {
		t.Errorf("expected provider fake, got %q", charge.Provider())
	}
	if charge.Status() != paymentsDomain.StatusPending {
		t.Errorf("expected StatusPending, got %s", charge.Status())
	}
}

func TestOrderShowTemplate_FailedAndPendingStatus(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)

	// 1. Test failed order
	failedOrder := checkoutQuery.NewOrderView(
		"ord-failed", "cust-1", checkoutDomain.StatusFailed, time.Now(),
		[]checkoutDomain.Line{
			checkoutDomain.NewLine("prod-1", "Go Mug", 1, 1500, "USD"),
		},
		checkoutDomain.Address{},
		checkoutDomain.ShippingMethod{},
		checkoutDomain.PaymentMethod{},
		1500, 0, 0, 1500, "USD", "", "", "", 0,
	)

	qrys := &mockCheckoutQrys{orders: map[string]checkoutQuery.OrderView{
		"ord-failed":  failedOrder,
		"ord-pending": checkoutQuery.NewOrderView(
			"ord-pending", "cust-1", checkoutDomain.StatusPending, time.Now(),
			[]checkoutDomain.Line{
				checkoutDomain.NewLine("prod-1", "Go Mug", 1, 1500, "USD"),
			},
			checkoutDomain.Address{},
			checkoutDomain.ShippingMethod{},
			checkoutDomain.PaymentMethod{},
			1500, 0, 0, 1500, "USD", "", "", "", 0,
		),
	}}

	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, qrys, nil)

	// Render failed order
	reqFailed := httptest.NewRequest(http.MethodGet, "/order/ord-failed", nil)
	reqFailed = mux.SetURLVars(reqFailed, map[string]string{"orderID": "ord-failed"})
	recFailed := httptest.NewRecorder()
	handler.Order(recFailed, reqFailed)

	if recFailed.Code != http.StatusOK {
		t.Fatalf("expected status 200 for failed order, got %d: %s", recFailed.Code, recFailed.Body.String())
	}
	failedBody := recFailed.Body.String()
	if !strings.Contains(failedBody, "payment declined.") {
		t.Errorf("failed order page missing 'payment declined.' title")
	}
	if !strings.Contains(failedBody, "Your payment was declined. The items have been released back to stock. You may retry your purchase or continue shopping.") {
		t.Errorf("failed order page missing decline notification banner")
	}
	if !strings.Contains(failedBody, "Return to Cart") {
		t.Errorf("failed order page missing 'Return to Cart' button")
	}
	if !strings.Contains(failedBody, "Continue Shopping") {
		t.Errorf("failed order page missing 'Continue Shopping' button")
	}

	// Render pending order
	reqPending := httptest.NewRequest(http.MethodGet, "/order/ord-pending", nil)
	reqPending = mux.SetURLVars(reqPending, map[string]string{"orderID": "ord-pending"})
	recPending := httptest.NewRecorder()
	handler.Order(recPending, reqPending)

	if recPending.Code != http.StatusOK {
		t.Fatalf("expected status 200 for pending order, got %d: %s", recPending.Code, recPending.Body.String())
	}
	pendingBody := recPending.Body.String()
	if !strings.Contains(pendingBody, "payment pending.") {
		t.Errorf("pending order page missing 'payment pending.' title")
	}
	if !strings.Contains(pendingBody, "Your order is pending payment confirmation.") {
		t.Errorf("pending order page missing pending notification banner")
	}
}

func TestCheckout_View_PaymentMethodsRadioChecked(t *testing.T) {
	setupFakePaymentEnv(t)
	storage := paymentsAdapter.NewInMemoryStorage()
	paySrv := paymentsApp.NewService(storage, nil, nil, nil)
	ctx := context.Background()

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-1", "Test Product", 1000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &mockCartSrv{cart: cart}

	handler := newFakePaymentTestHandler(paySrv, &mockCheckoutCmds{}, &mockCheckoutQrys{}, cartSrv)

	// 1. When Stripe is enabled: card is first and checked
	req := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	rec := httptest.NewRecorder()
	handler.Checkout(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="card" checked`) {
		t.Errorf("expected card to be checked by default, got: %s", body)
	}

	// 2. When Stripe is disabled: fake is first and checked
	if err := paySrv.UpdateProvider(ctx, paymentsDomain.ProviderStripe, false, nil); err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	rec2 := httptest.NewRecorder()
	handler.Checkout(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec2.Code)
	}
	body2 := rec2.Body.String()
	if strings.Contains(body2, `value="card"`) {
		t.Errorf("expected card to be omitted when disabled")
	}
	if !strings.Contains(body2, `value="paypal" checked`) {
		t.Errorf("expected paypal to be first and checked, got: %s", body2)
	}
}
