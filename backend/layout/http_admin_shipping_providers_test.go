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

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type mockShippingCheckoutCmds struct {
	methods                []checkoutDomain.ShippingMethod
	updateShippingMethodFn func(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error
	updateTrackingFn       func(ctx context.Context, orderID, carrier, trackingCode string) error
}

func (m *mockShippingCheckoutCmds) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error) {
	return checkoutDomain.Order{}, nil
}

func (m *mockShippingCheckoutCmds) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}

func (m *mockShippingCheckoutCmds) AdminCancel(ctx context.Context, orderID string) error {
	return nil
}

func (m *mockShippingCheckoutCmds) MarkPaid(ctx context.Context, orderID string) error {
	return nil
}

func (m *mockShippingCheckoutCmds) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	return nil
}

func (m *mockShippingCheckoutCmds) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return m.methods, nil
}

func (m *mockShippingCheckoutCmds) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	for _, sm := range m.methods {
		if sm.Code() == code {
			return sm, nil
		}
	}
	return checkoutDomain.ShippingMethod{}, errors.New("method not found")
}

func (m *mockShippingCheckoutCmds) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	if m.updateShippingMethodFn != nil {
		return m.updateShippingMethodFn(ctx, code, enabled, label, cost, carrier)
	}
	return nil
}

func (m *mockShippingCheckoutCmds) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	if m.updateTrackingFn != nil {
		return m.updateTrackingFn(ctx, orderID, carrier, trackingCode)
	}
	return nil
}

func newTestShippingHandler(checkoutCmds checkoutCommands) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	return httpHandler{
		checkoutSrv:  checkoutCmds,
		adminAuthSrv: adminAuth,
		rates:        rates,
		logger:       logger,
	}
}

func TestAdminShippingProviders_RequiresAdmin(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestShippingHandler(&mockShippingCheckoutCmds{})

	req := httptest.NewRequest(http.MethodGet, "/admin/shipping-providers", nil)
	rec := httptest.NewRecorder()

	handler.AdminShippingProviders(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("expected redirect to /admin/login, got %s", loc)
	}
}

func TestAdminShippingProviders_RendersProviders(t *testing.T) {
	setupTestEnvironment(t)

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
		checkoutDomain.NewShippingMethod("pickup", "Personal pickup", 0, false, "Store Pickup", false),
		checkoutDomain.NewShippingMethod("courier", "Express Courier", 1500, true, "DHL", true),
	}

	mockCmds := &mockShippingCheckoutCmds{methods: methods}
	handler := newTestShippingHandler(mockCmds)

	req := httptest.NewRequest(http.MethodGet, "/admin/shipping-providers", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminShippingProviders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, expected := range []string{
		"shipping providers",
		"Configure available shipping methods, pricing, and carrier settings.",
		"Flat rate",
		"5.00",
		"Standard Post",
		"Personal pickup",
		"0.00",
		"Store Pickup",
		"Express Courier",
		"15.00",
		"DHL",
		"enabled",
		"disabled",
		"/admin/shipping-providers/flat",
		"/admin/shipping-providers/pickup",
		"/admin/shipping-providers/courier",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected body to contain %q, but it didn't", expected)
		}
	}
}

func TestAdminUpdateShippingProvider_RequiresAdmin(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestShippingHandler(&mockShippingCheckoutCmds{})

	req := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/flat", nil)
	rec := httptest.NewRecorder()

	handler.AdminUpdateShippingProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("expected redirect to /admin/login, got %s", loc)
	}
}

func TestAdminUpdateShippingProvider_Success(t *testing.T) {
	setupTestEnvironment(t)

	var updatedCode string
	var updatedEnabled bool
	var updatedLabel string
	var updatedCost int64
	var updatedCarrier string

	mockCmds := &mockShippingCheckoutCmds{
		updateShippingMethodFn: func(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
			updatedCode = code
			updatedEnabled = enabled
			updatedLabel = label
			updatedCost = cost
			updatedCarrier = carrier
			return nil
		},
	}
	handler := newTestShippingHandler(mockCmds)

	form := url.Values{
		"enabled": {"1"},
		"label":   {"Standard Post"},
		"cost":    {"7.50"},
		"carrier": {"USPS"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/flat", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"code": "flat"})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateShippingProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/shipping-providers" {
		t.Fatalf("expected redirect to /admin/shipping-providers, got %s", loc)
	}

	if updatedCode != "flat" {
		t.Errorf("expected updatedCode to be 'flat', got %q", updatedCode)
	}
	if !updatedEnabled {
		t.Errorf("expected updatedEnabled to be true, got false")
	}
	if updatedLabel != "Standard Post" {
		t.Errorf("expected updatedLabel to be 'Standard Post', got %q", updatedLabel)
	}
	if updatedCost != 750 {
		t.Errorf("expected updatedCost to be 750, got %d", updatedCost)
	}
	if updatedCarrier != "USPS" {
		t.Errorf("expected updatedCarrier to be 'USPS', got %q", updatedCarrier)
	}

	// Verify flash message
	s, _ := store.Get(req, "ecommerce")
	flashes := s.Flashes()
	if len(flashes) == 0 || !strings.Contains(flashes[0].(string), "Shipping provider updated successfully.") {
		t.Errorf("expected success flash message, got %v", flashes)
	}
}

func TestAdminUpdateShippingProvider_PickupDefaultsZeroCost(t *testing.T) {
	setupTestEnvironment(t)

	var updatedCode string
	var updatedCost int64

	mockCmds := &mockShippingCheckoutCmds{
		updateShippingMethodFn: func(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
			updatedCode = code
			updatedCost = cost
			return nil
		},
	}
	handler := newTestShippingHandler(mockCmds)

	form := url.Values{
		"enabled": {"1"},
		"label":   {"Local Pickup"},
		"cost":    {""},
		"carrier": {"Store"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/pickup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"code": "pickup"})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateShippingProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/shipping-providers" {
		t.Fatalf("expected redirect to /admin/shipping-providers, got %s", loc)
	}

	if updatedCode != "pickup" {
		t.Errorf("expected updatedCode to be 'pickup', got %q", updatedCode)
	}
	if updatedCost != 0 {
		t.Errorf("expected updatedCost to be 0 for empty pickup cost, got %d", updatedCost)
	}
}

func TestAdminUpdateShippingProvider_InvalidCost(t *testing.T) {
	setupTestEnvironment(t)

	called := false
	mockCmds := &mockShippingCheckoutCmds{
		updateShippingMethodFn: func(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
			called = true
			return nil
		},
	}
	handler := newTestShippingHandler(mockCmds)

	form := url.Values{
		"enabled": {"1"},
		"label":   {"Flat rate"},
		"cost":    {"not-a-number"},
		"carrier": {"Post"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/shipping-providers/flat", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"code": "flat"})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdateShippingProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/shipping-providers" {
		t.Fatalf("expected redirect to /admin/shipping-providers, got %s", loc)
	}
	if called {
		t.Errorf("expected UpdateShippingMethod not to be called for invalid cost")
	}

	// Verify error flash
	s, _ := store.Get(req, "ecommerce")
	flashes := s.Flashes("error")
	if len(flashes) == 0 {
		t.Errorf("expected error flash message for invalid cost, got none")
	}
}
