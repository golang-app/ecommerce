package layout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	paymentsDomain "github.com/bkielbasa/go-ecommerce/backend/payments/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type mockPaymentsService struct {
	providers        []paymentsDomain.ProviderConfig
	updateProviderFn func(ctx context.Context, id string, enabled bool, config map[string]string) error
}

func (m *mockPaymentsService) ListProviders(ctx context.Context) ([]paymentsDomain.ProviderConfig, error) {
	return m.providers, nil
}

func (m *mockPaymentsService) FindProvider(ctx context.Context, id string) (paymentsDomain.ProviderConfig, error) {
	for _, p := range m.providers {
		if p.ID() == id {
			return p, nil
		}
	}
	return paymentsDomain.ProviderConfig{}, errors.New("provider not found")
}

func (m *mockPaymentsService) UpdateProvider(ctx context.Context, id string, enabled bool, config map[string]string) error {
	if m.updateProviderFn != nil {
		return m.updateProviderFn(ctx, id, enabled, config)
	}
	return nil
}

func (m *mockPaymentsService) CreatePendingCharge(ctx context.Context, orderID string, amount int64, currency, provider string) (paymentsDomain.Charge, error) {
	return paymentsDomain.Charge{}, nil
}

func (m *mockPaymentsService) ConfirmCharge(ctx context.Context, chargeID string) (paymentsDomain.Charge, error) {
	return paymentsDomain.Charge{}, nil
}

func (m *mockPaymentsService) RejectCharge(ctx context.Context, chargeID string, reason string) (paymentsDomain.Charge, error) {
	return paymentsDomain.Charge{}, nil
}

func (m *mockPaymentsService) FindByOrderID(ctx context.Context, orderID string) (paymentsDomain.Charge, error) {
	return paymentsDomain.Charge{}, nil
}

func (m *mockPaymentsService) FindCharge(ctx context.Context, id string) (paymentsDomain.Charge, error) {
	return paymentsDomain.Charge{}, nil
}

func (m *mockPaymentsService) MarkSucceeded(ctx context.Context, providerRef string) error {
	return nil
}

func (m *mockPaymentsService) MarkFailed(ctx context.Context, providerRef, reason string) error {
	return nil
}

type fakeAdminAuthService struct {
	admin authAdapter.Admin
	sess  *authDomain.Session
}

func (f fakeAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return f.sess, nil
}
func (f fakeAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f fakeAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return f.sess, nil
}
func (f fakeAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f fakeAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f fakeAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return f.admin, nil
}

func setupTestEnvironment(t *testing.T) {
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
	ResetRateLimiters()
}

func setAdminSession(t *testing.T, req *http.Request) {
	session, err := store.Get(req, adminSessionCookie)
	if err != nil {
		t.Fatalf("failed to get admin session: %v", err)
	}
	session.Values[adminSessionIDKey] = "admin-session-token"
	rec := httptest.NewRecorder()
	if err := session.Save(req, rec); err != nil {
		t.Fatalf("failed to save admin session: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
}

func newTestHandler(paySrv *mockPaymentsService) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	return httpHandler{
		paymentsSrv:  paySrv,
		adminAuthSrv: adminAuth,
		rates:        rates,
		logger:       logger,
	}
}

func TestAdminPaymentProviders_RequiresAdmin(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestHandler(&mockPaymentsService{})

	req := httptest.NewRequest(http.MethodGet, "/admin/payment-providers", nil)
	rec := httptest.NewRecorder()

	handler.AdminPaymentProviders(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("expected redirect to /admin/login, got %s", loc)
	}
}

func TestAdminPaymentProviders_RendersProviders(t *testing.T) {
	setupTestEnvironment(t)

	now := time.Now().UTC()
	stripeCfg := paymentsDomain.NewProviderConfig(
		"stripe",
		true,
		map[string]string{
			"fail_card_ending_in": "0000",
			"publishable_key":     "pk_test_12345",
			"secret_key":          "sk_test_67890",
			"webhook_secret":      "whsec_test_secret",
		},
		now,
	)
	fakeCfg := paymentsDomain.NewProviderConfig(
		"fake",
		false,
		map[string]string{
			"name":        "Fake Payment Simulator",
			"description": "Interactive test simulator",
		},
		now,
	)

	mockSrv := &mockPaymentsService{
		providers: []paymentsDomain.ProviderConfig{stripeCfg, fakeCfg},
	}
	handler := newTestHandler(mockSrv)

	req := httptest.NewRequest(http.MethodGet, "/admin/payment-providers", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminPaymentProviders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, expected := range []string{
		"payment providers",
		"Stripe",
		"Fake Payment Simulator",
		"pk_test_12345",
		"whsec_test_secret",
		"Interactive test simulator",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("expected body to contain %q, but it didn't", expected)
		}
	}
}

func TestAdminUpdatePaymentProvider_RequiresAdmin(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestHandler(&mockPaymentsService{})

	req := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/stripe", nil)
	rec := httptest.NewRecorder()

	handler.AdminUpdatePaymentProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/login" {
		t.Fatalf("expected redirect to /admin/login, got %s", loc)
	}
}

func TestAdminUpdatePaymentProvider_Stripe(t *testing.T) {
	setupTestEnvironment(t)

	var updatedID string
	var updatedEnabled bool
	var updatedConfig map[string]string

	mockSrv := &mockPaymentsService{
		updateProviderFn: func(ctx context.Context, id string, enabled bool, config map[string]string) error {
			updatedID = id
			updatedEnabled = enabled
			updatedConfig = config
			return nil
		},
	}
	handler := newTestHandler(mockSrv)

	form := url.Values{
		"enabled":             {"1"},
		"fail_card_ending_in": {"4242"},
		"publishable_key":     {"pk_live_stripe"},
		"secret_key":          {"sk_live_stripe"},
		"webhook_secret":      {"whsec_live_stripe"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/stripe", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"id": "stripe"})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdatePaymentProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/payment-providers" {
		t.Fatalf("expected redirect to /admin/payment-providers, got %s", loc)
	}

	if updatedID != "stripe" {
		t.Errorf("expected updatedID to be 'stripe', got %q", updatedID)
	}
	if !updatedEnabled {
		t.Errorf("expected updatedEnabled to be true, got false")
	}
	if updatedConfig["fail_card_ending_in"] != "4242" {
		t.Errorf("expected fail_card_ending_in to be '4242', got %q", updatedConfig["fail_card_ending_in"])
	}
	if updatedConfig["publishable_key"] != "pk_live_stripe" {
		t.Errorf("expected publishable_key to be 'pk_live_stripe', got %q", updatedConfig["publishable_key"])
	}
	if updatedConfig["secret_key"] != "sk_live_stripe" {
		t.Errorf("expected secret_key to be 'sk_live_stripe', got %q", updatedConfig["secret_key"])
	}
	if updatedConfig["webhook_secret"] != "whsec_live_stripe" {
		t.Errorf("expected webhook_secret to be 'whsec_live_stripe', got %q", updatedConfig["webhook_secret"])
	}

	// Verify flash message
	s, _ := store.Get(req, "ecommerce")
	flashes := s.Flashes()
	if len(flashes) == 0 || !strings.Contains(flashes[0].(string), "Payment provider updated successfully.") {
		t.Errorf("expected success flash message, got %v", flashes)
	}
}

func TestAdminUpdatePaymentProvider_Fake(t *testing.T) {
	setupTestEnvironment(t)

	var updatedID string
	var updatedEnabled bool
	var updatedConfig map[string]string

	mockSrv := &mockPaymentsService{
		updateProviderFn: func(ctx context.Context, id string, enabled bool, config map[string]string) error {
			updatedID = id
			updatedEnabled = enabled
			updatedConfig = config
			return nil
		},
	}
	handler := newTestHandler(mockSrv)

	form := url.Values{
		"name":        {"Custom Fake Simulator"},
		"description": {"Custom simulator description"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/payment-providers/fake", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = mux.SetURLVars(req, map[string]string{"id": "fake"})
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminUpdatePaymentProvider(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/payment-providers" {
		t.Fatalf("expected redirect to /admin/payment-providers, got %s", loc)
	}

	if updatedID != "fake" {
		t.Errorf("expected updatedID to be 'fake', got %q", updatedID)
	}
	if updatedEnabled {
		t.Errorf("expected updatedEnabled to be false, got true")
	}
	if updatedConfig["name"] != "Custom Fake Simulator" {
		t.Errorf("expected name to be 'Custom Fake Simulator', got %q", updatedConfig["name"])
	}
	if updatedConfig["description"] != "Custom simulator description" {
		t.Errorf("expected description to be 'Custom simulator description', got %q", updatedConfig["description"])
	}
}
