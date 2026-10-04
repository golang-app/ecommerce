package layout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
	wishlistDomain "github.com/bkielbasa/go-ecommerce/backend/wishlist/domain"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// --- Mock implementations for Customer Admin testing ---

type mockAdminCustomerAuthSrv struct {
	listCustomersFn        func(ctx context.Context) ([]string, error)
	requestPasswordResetFn func(ctx context.Context, email string) (string, error)
}

func (m *mockAdminCustomerAuthSrv) Login(ctx context.Context, username string, password string) (*authDomain.Session, error) {
	return nil, nil
}
func (m *mockAdminCustomerAuthSrv) Logout(ctx context.Context, seesionID string) error {
	return nil
}
func (m *mockAdminCustomerAuthSrv) CreateNewCustomer(ctx context.Context, email, password string) error {
	return nil
}
func (m *mockAdminCustomerAuthSrv) FindByToken(ctx context.Context, sessToken string) (*authDomain.Session, error) {
	return nil, nil
}
func (m *mockAdminCustomerAuthSrv) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (m *mockAdminCustomerAuthSrv) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	if m.requestPasswordResetFn != nil {
		return m.requestPasswordResetFn(ctx, email)
	}
	return "", nil
}
func (m *mockAdminCustomerAuthSrv) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	return nil
}
func (m *mockAdminCustomerAuthSrv) ListCustomers(ctx context.Context) ([]string, error) {
	if m.listCustomersFn != nil {
		return m.listCustomersFn(ctx)
	}
	return nil, nil
}

type mockAdminCustomerCheckoutQry struct {
	listCustomerOrderStatsFn func(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error)
	listByCustomerFn         func(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error)
}

func (m *mockAdminCustomerCheckoutQry) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	return checkoutQuery.OrderView{}, nil
}
func (m *mockAdminCustomerCheckoutQry) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	if m.listByCustomerFn != nil {
		return m.listByCustomerFn(ctx, customerID)
	}
	return nil, nil
}
func (m *mockAdminCustomerCheckoutQry) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}
func (m *mockAdminCustomerCheckoutQry) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}
func (m *mockAdminCustomerCheckoutQry) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}
func (m *mockAdminCustomerCheckoutQry) ListCustomerOrderStats(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
	if m.listCustomerOrderStatsFn != nil {
		return m.listCustomerOrderStatsFn(ctx)
	}
	return nil, nil
}

type mockAdminCustomerShippingSrv struct {
	listFn func(ctx context.Context, customerID string) ([]shipDomain.Address, error)
}

func (m *mockAdminCustomerShippingSrv) List(ctx context.Context, customerID string) ([]shipDomain.Address, error) {
	if m.listFn != nil {
		return m.listFn(ctx, customerID)
	}
	return nil, nil
}
func (m *mockAdminCustomerShippingSrv) Get(ctx context.Context, customerID, id string) (shipDomain.Address, error) {
	return shipDomain.Address{}, nil
}
func (m *mockAdminCustomerShippingSrv) Add(ctx context.Context, customerID, name, street1, street2, city, zip, country string) error {
	return nil
}
func (m *mockAdminCustomerShippingSrv) Edit(ctx context.Context, customerID, id, name, street1, street2, city, zip, country string) error {
	return nil
}
func (m *mockAdminCustomerShippingSrv) Remove(ctx context.Context, customerID, id string) error {
	return nil
}
func (m *mockAdminCustomerShippingSrv) SetDefault(ctx context.Context, customerID, id string) error {
	return nil
}
func (m *mockAdminCustomerShippingSrv) Default(ctx context.Context, customerID string) (shipDomain.Address, bool, error) {
	return shipDomain.Address{}, false, nil
}

type mockAdminCustomerReviewsSrv struct {
	listByCustomerFn func(ctx context.Context, customerID string) ([]reviewsDomain.Review, error)
}

func (m *mockAdminCustomerReviewsSrv) ListForProduct(ctx context.Context, productID string, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (m *mockAdminCustomerReviewsSrv) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]reviewsDomain.Aggregate, error) {
	return nil, nil
}
func (m *mockAdminCustomerReviewsSrv) HasReviewed(ctx context.Context, productID, customerID string) (bool, error) {
	return false, nil
}
func (m *mockAdminCustomerReviewsSrv) Submit(ctx context.Context, productID, customerID, body string, rating int) error {
	return nil
}
func (m *mockAdminCustomerReviewsSrv) Delete(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdminCustomerReviewsSrv) Approve(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdminCustomerReviewsSrv) Reject(ctx context.Context, id string) error {
	return nil
}
func (m *mockAdminCustomerReviewsSrv) ListPending(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (m *mockAdminCustomerReviewsSrv) ListAll(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (m *mockAdminCustomerReviewsSrv) ListByCustomer(ctx context.Context, customerID string) ([]reviewsDomain.Review, error) {
	if m.listByCustomerFn != nil {
		return m.listByCustomerFn(ctx, customerID)
	}
	return nil, nil
}

type mockAdminCustomerWishlistSrv struct {
	listByCustomerFn func(ctx context.Context, customerID string) ([]wishlistDomain.Item, error)
}

func (m *mockAdminCustomerWishlistSrv) Toggle(ctx context.Context, customerID, variantID string) (bool, error) {
	return false, nil
}
func (m *mockAdminCustomerWishlistSrv) ListByCustomer(ctx context.Context, customerID string) ([]wishlistDomain.Item, error) {
	if m.listByCustomerFn != nil {
		return m.listByCustomerFn(ctx, customerID)
	}
	return nil, nil
}
func (m *mockAdminCustomerWishlistSrv) Contains(ctx context.Context, customerID, variantID string) (bool, error) {
	return false, nil
}

type mockAdminCustomerMailer struct {
	sent []mailer.Message
}

func (m *mockAdminCustomerMailer) Send(ctx context.Context, msg mailer.Message) error {
	m.sent = append(m.sent, msg)
	return nil
}

func newTestCustomerAdminHandler(
	authSrv authService,
	checkoutQry checkoutQueries,
	shipSrv shippingService,
	reviewsSrv reviewsService,
	wishlistSrv wishlistService,
	mailerSrv mailer.Mailer,
) httpHandler {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	return httpHandler{
		authSrv:      authSrv,
		adminAuthSrv: adminAuth,
		checkoutQry:  checkoutQry,
		shipSrv:      shipSrv,
		reviewsSrv:   reviewsSrv,
		wishlistSrv:  wishlistSrv,
		mailer:       mailerSrv,
		baseURL:      "http://localhost:8080",
		rates:        rates,
		logger:       logger,
	}
}

// 1. Auth required: anonymous / non-admin request redirected to /admin/login
func TestAdminCustomers_AuthRequired(t *testing.T) {
	setupTestEnvironment(t)
	handler := newTestCustomerAdminHandler(
		&mockAdminCustomerAuthSrv{},
		&mockAdminCustomerCheckoutQry{},
		&mockAdminCustomerShippingSrv{},
		&mockAdminCustomerReviewsSrv{},
		&mockAdminCustomerWishlistSrv{},
		&mockAdminCustomerMailer{},
	)

	endpoints := []struct {
		method string
		target string
		vars   map[string]string
		fn     func(w http.ResponseWriter, r *http.Request)
	}{
		{
			method: http.MethodGet,
			target: "/admin/customers",
			fn:     handler.AdminCustomers,
		},
		{
			method: http.MethodGet,
			target: "/admin/customers/alice@example.com",
			vars:   map[string]string{"email": "alice@example.com"},
			fn:     handler.AdminCustomerDetail,
		},
		{
			method: http.MethodPost,
			target: "/admin/customers/alice@example.com/reset-password",
			vars:   map[string]string{"email": "alice@example.com"},
			fn:     handler.AdminCustomerTriggerPasswordReset,
		},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.target, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.target, nil)
			if ep.vars != nil {
				req = mux.SetURLVars(req, ep.vars)
			}
			rec := httptest.NewRecorder()

			ep.fn(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("expected status 303, got %d", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "/admin/login" {
				t.Fatalf("expected redirect to /admin/login, got %s", loc)
			}
		})
	}
}

// 2. Admin sees combined registered users and guest purchasers with correct badges
func TestAdminCustomers_ListAndMerge(t *testing.T) {
	setupTestEnvironment(t)

	now := time.Now()
	authMock := &mockAdminCustomerAuthSrv{
		listCustomersFn: func(ctx context.Context) ([]string, error) {
			return []string{"alice@example.com", "bob@example.com"}, nil
		},
	}

	checkoutMock := &mockAdminCustomerCheckoutQry{
		listCustomerOrderStatsFn: func(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
			return []checkoutQuery.CustomerOrderStat{
				checkoutQuery.NewCustomerOrderStat("bob@example.com", "Bob Builder", 2, 5000, "USD", now),
				checkoutQuery.NewCustomerOrderStat("charlie@guest.com", "Charlie Guest", 1, 2500, "USD", now.Add(-time.Hour)),
			}, nil
		},
	}

	handler := newTestCustomerAdminHandler(
		authMock,
		checkoutMock,
		&mockAdminCustomerShippingSrv{},
		&mockAdminCustomerReviewsSrv{},
		&mockAdminCustomerWishlistSrv{},
		&mockAdminCustomerMailer{},
	)

	req := httptest.NewRequest(http.MethodGet, "/admin/customers", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	handler.AdminCustomers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()

	// Registered customer with 0 orders
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("expected body to contain alice@example.com")
	}

	// Registered customer with orders & name
	if !strings.Contains(body, "bob@example.com") {
		t.Errorf("expected body to contain bob@example.com")
	}
	if !strings.Contains(body, "Bob Builder") {
		t.Errorf("expected body to contain Bob Builder")
	}

	// Guest purchaser
	if !strings.Contains(body, "charlie@guest.com") {
		t.Errorf("expected body to contain charlie@guest.com")
	}
	if !strings.Contains(body, "Charlie Guest") {
		t.Errorf("expected body to contain Charlie Guest")
	}

	// Badges
	if !strings.Contains(body, "Registered") {
		t.Errorf("expected body to contain 'Registered' badge")
	}
	if !strings.Contains(body, "Guest") {
		t.Errorf("expected body to contain 'Guest' badge")
	}
}

// 3. Search and filter
func TestAdminCustomers_SearchAndFilter(t *testing.T) {
	setupTestEnvironment(t)

	now := time.Now()
	authMock := &mockAdminCustomerAuthSrv{
		listCustomersFn: func(ctx context.Context) ([]string, error) {
			return []string{"alice@example.com", "bob@example.com"}, nil
		},
	}

	checkoutMock := &mockAdminCustomerCheckoutQry{
		listCustomerOrderStatsFn: func(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
			return []checkoutQuery.CustomerOrderStat{
				checkoutQuery.NewCustomerOrderStat("bob@example.com", "Bob Builder", 2, 5000, "USD", now),
				checkoutQuery.NewCustomerOrderStat("charlie@guest.com", "Charlie Guest", 1, 2500, "USD", now.Add(-time.Hour)),
			}, nil
		},
	}

	handler := newTestCustomerAdminHandler(
		authMock,
		checkoutMock,
		&mockAdminCustomerShippingSrv{},
		&mockAdminCustomerReviewsSrv{},
		&mockAdminCustomerWishlistSrv{},
		&mockAdminCustomerMailer{},
	)

	t.Run("search by email", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers?q=guest.com", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomers(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "charlie@guest.com") {
			t.Errorf("expected body to contain charlie@guest.com")
		}
		if strings.Contains(body, "alice@example.com") {
			t.Errorf("expected body NOT to contain alice@example.com")
		}
		if strings.Contains(body, "bob@example.com") {
			t.Errorf("expected body NOT to contain bob@example.com")
		}
	})

	t.Run("search by shipping name", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers?q=Builder", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomers(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "bob@example.com") {
			t.Errorf("expected body to contain bob@example.com")
		}
		if strings.Contains(body, "alice@example.com") {
			t.Errorf("expected body NOT to contain alice@example.com")
		}
		if strings.Contains(body, "charlie@guest.com") {
			t.Errorf("expected body NOT to contain charlie@guest.com")
		}
	})

	t.Run("filter registered", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers?filter=registered", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomers(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "alice@example.com") {
			t.Errorf("expected body to contain alice@example.com")
		}
		if !strings.Contains(body, "bob@example.com") {
			t.Errorf("expected body to contain bob@example.com")
		}
		if strings.Contains(body, "charlie@guest.com") {
			t.Errorf("expected body NOT to contain charlie@guest.com")
		}
	})

	t.Run("filter guest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers?filter=guest", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomers(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "charlie@guest.com") {
			t.Errorf("expected body to contain charlie@guest.com")
		}
		if strings.Contains(body, "alice@example.com") {
			t.Errorf("expected body NOT to contain alice@example.com")
		}
		if strings.Contains(body, "bob@example.com") {
			t.Errorf("expected body NOT to contain bob@example.com")
		}
	})
}

// 4. Detail view rendering
func TestAdminCustomerDetail_Render(t *testing.T) {
	setupTestEnvironment(t)

	now := time.Now()
	authMock := &mockAdminCustomerAuthSrv{
		listCustomersFn: func(ctx context.Context) ([]string, error) {
			return []string{"alice@example.com"}, nil
		},
	}

	checkoutMock := &mockAdminCustomerCheckoutQry{
		listByCustomerFn: func(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
			if customerID == "alice@example.com" {
				return []checkoutQuery.OrderSummary{
					checkoutQuery.NewOrderSummary(customerID, "ord-101", checkoutDomain.StatusPaid, now, 2, 4000, "USD"),
				}, nil
			}
			return nil, nil
		},
	}

	shipMock := &mockAdminCustomerShippingSrv{
		listFn: func(ctx context.Context, customerID string) ([]shipDomain.Address, error) {
			if customerID == "alice@example.com" {
				addr, _ := shipDomain.NewAddress("addr-1", customerID, "Alice Smith", "10 Downing St", "", "London", "SW1A 2AA", "UK", true, now)
				return []shipDomain.Address{addr}, nil
			}
			return nil, nil
		},
	}

	revMock := &mockAdminCustomerReviewsSrv{
		listByCustomerFn: func(ctx context.Context, customerID string) ([]reviewsDomain.Review, error) {
			if customerID == "alice@example.com" {
				rev, _ := reviewsDomain.NewReview("rev-1", "prod-abc", customerID, "Loved it!", 5, now, reviewsDomain.StatusApproved)
				return []reviewsDomain.Review{rev}, nil
			}
			return nil, nil
		},
	}

	wishMock := &mockAdminCustomerWishlistSrv{
		listByCustomerFn: func(ctx context.Context, customerID string) ([]wishlistDomain.Item, error) {
			if customerID == "alice@example.com" {
				return []wishlistDomain.Item{
					wishlistDomain.Rebuild(customerID, "var-xyz", now),
				}, nil
			}
			return nil, nil
		},
	}

	handler := newTestCustomerAdminHandler(
		authMock,
		checkoutMock,
		shipMock,
		revMock,
		wishMock,
		&mockAdminCustomerMailer{},
	)

	t.Run("registered customer detail with all sections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/alice@example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"email": "alice@example.com"})
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomerDetail(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		body := rec.Body.String()
		if !strings.Contains(body, "alice@example.com") {
			t.Errorf("expected body to contain customer email alice@example.com")
		}
		if !strings.Contains(body, "Registered Account") {
			t.Errorf("expected body to contain Registered Account badge")
		}
		if !strings.Contains(body, "Send Password Reset Email") {
			t.Errorf("expected body to contain password reset button for registered customer")
		}
		if !strings.Contains(body, "ord-101") {
			t.Errorf("expected body to contain order id ord-101")
		}
		if !strings.Contains(body, "10 Downing St") {
			t.Errorf("expected body to contain address 10 Downing St")
		}
		if !strings.Contains(body, "Loved it!") {
			t.Errorf("expected body to contain review comment Loved it!")
		}
		if !strings.Contains(body, "var-xyz") {
			t.Errorf("expected body to contain wishlist variant var-xyz")
		}
	})

	t.Run("guest customer detail without password reset button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/guest@example.com", nil)
		req = mux.SetURLVars(req, map[string]string{"email": "guest@example.com"})
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomerDetail(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		body := rec.Body.String()
		if !strings.Contains(body, "guest@example.com") {
			t.Errorf("expected body to contain guest@example.com")
		}
		if !strings.Contains(body, "Guest Purchaser") {
			t.Errorf("expected body to contain Guest Purchaser badge")
		}
		if strings.Contains(body, "Send Password Reset Email") {
			t.Errorf("expected body NOT to contain password reset button for guest customer")
		}
	})
}

// 5. Password reset trigger: POST triggers reset for registered users and rejects guest users
func TestAdminCustomer_PasswordReset(t *testing.T) {
	setupTestEnvironment(t)

	var requestedEmail string
	authMock := &mockAdminCustomerAuthSrv{
		listCustomersFn: func(ctx context.Context) ([]string, error) {
			return []string{"alice@example.com"}, nil
		},
		requestPasswordResetFn: func(ctx context.Context, email string) (string, error) {
			requestedEmail = email
			return "valid-raw-reset-token", nil
		},
	}

	mailerMock := &mockAdminCustomerMailer{}

	handler := newTestCustomerAdminHandler(
		authMock,
		&mockAdminCustomerCheckoutQry{},
		&mockAdminCustomerShippingSrv{},
		&mockAdminCustomerReviewsSrv{},
		&mockAdminCustomerWishlistSrv{},
		mailerMock,
	)

	t.Run("registered user password reset sends email and redirects", func(t *testing.T) {
		requestedEmail = ""
		mailerMock.sent = nil

		req := httptest.NewRequest(http.MethodPost, "/admin/customers/alice@example.com/reset-password", nil)
		req = mux.SetURLVars(req, map[string]string{"email": "alice@example.com"})
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomerTriggerPasswordReset(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected status 303, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/customers/alice@example.com" {
			t.Fatalf("expected redirect to /admin/customers/alice@example.com, got %s", loc)
		}
		if requestedEmail != "alice@example.com" {
			t.Fatalf("expected RequestPasswordReset called for alice@example.com, got %s", requestedEmail)
		}
		if len(mailerMock.sent) != 1 {
			t.Fatalf("expected 1 email sent, got %d", len(mailerMock.sent))
		}
		if mailerMock.sent[0].To != "alice@example.com" {
			t.Fatalf("expected email sent to alice@example.com, got %s", mailerMock.sent[0].To)
		}
		if !strings.Contains(mailerMock.sent[0].TextBody, "valid-raw-reset-token") {
			t.Errorf("expected email body to contain reset token")
		}

		// Verify flash message
		s, err := store.Get(req, "ecommerce")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		flashes := s.Flashes()
		if len(flashes) == 0 {
			t.Errorf("expected flash message to be set")
		}
	})

	t.Run("guest user password reset is rejected", func(t *testing.T) {
		requestedEmail = ""
		mailerMock.sent = nil

		req := httptest.NewRequest(http.MethodPost, "/admin/customers/guest@example.com/reset-password", nil)
		req = mux.SetURLVars(req, map[string]string{"email": "guest@example.com"})
		setAdminSession(t, req)
		rec := httptest.NewRecorder()

		handler.AdminCustomerTriggerPasswordReset(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected status 303, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/customers/guest@example.com" {
			t.Fatalf("expected redirect to /admin/customers/guest@example.com, got %s", loc)
		}
		if requestedEmail != "" {
			t.Fatalf("expected RequestPasswordReset NOT called for guest, but got %s", requestedEmail)
		}
		if len(mailerMock.sent) != 0 {
			t.Fatalf("expected 0 emails sent for guest, got %d", len(mailerMock.sent))
		}

		// Verify error flash message
		s, err := store.Get(req, "ecommerce")
		if err != nil {
			t.Fatalf("failed to get session: %v", err)
		}
		errorFlashes := s.Flashes("error")
		if len(errorFlashes) == 0 {
			t.Errorf("expected error flash message to be set")
		}
	})
}

// 6. Router registration verification
func TestAdminCustomers_Routing(t *testing.T) {
	setupTestEnvironment(t)

	authMock := &mockAdminCustomerAuthSrv{
		listCustomersFn: func(ctx context.Context) ([]string, error) {
			return []string{"alice@example.com"}, nil
		},
		requestPasswordResetFn: func(ctx context.Context, email string) (string, error) {
			return "tok-123", nil
		},
	}
	checkoutMock := &mockAdminCustomerCheckoutQry{}
	mailerMock := &mockAdminCustomerMailer{}

	handler := newTestCustomerAdminHandler(
		authMock,
		checkoutMock,
		&mockAdminCustomerShippingSrv{},
		&mockAdminCustomerReviewsSrv{},
		&mockAdminCustomerWishlistSrv{},
		mailerMock,
	)

	bc := boundedContext{
		handler: handler,
		logger:  handler.logger,
	}

	router := mux.NewRouter()
	bc.MuxRegister(router)

	t.Run("GET /admin/customers route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("GET /admin/customers/{email} route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/alice@example.com", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("POST /admin/customers/{email}/reset-password route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/customers/alice@example.com/reset-password", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected status 303, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/customers/alice@example.com" {
			t.Fatalf("expected redirect to /admin/customers/alice@example.com, got %s", loc)
		}
	})
}

