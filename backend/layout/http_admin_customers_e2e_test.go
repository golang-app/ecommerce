package layout

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authApp "github.com/bkielbasa/go-ecommerce/backend/auth/app"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	reviewsAdapter "github.com/bkielbasa/go-ecommerce/backend/reviews/adapter"
	reviewsApp "github.com/bkielbasa/go-ecommerce/backend/reviews/app"
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	shippingApp "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/app"
	shipDomain "github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
	wishlistAdapter "github.com/bkielbasa/go-ecommerce/backend/wishlist/adapter"
	wishlistApp "github.com/bkielbasa/go-ecommerce/backend/wishlist/app"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// adminCustomersE2EShippingStorage implements shippingApp.Storage in-memory.
type adminCustomersE2EShippingStorage struct {
	mu        sync.Mutex
	addresses map[string][]shipDomain.Address
}

func newAdminCustomersE2EShippingStorage() *adminCustomersE2EShippingStorage {
	return &adminCustomersE2EShippingStorage{
		addresses: make(map[string][]shipDomain.Address),
	}
}

func (s *adminCustomersE2EShippingStorage) List(ctx context.Context, customerID string) ([]shipDomain.Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]shipDomain.Address(nil), s.addresses[customerID]...), nil
}

func (s *adminCustomersE2EShippingStorage) Get(ctx context.Context, customerID, id string) (shipDomain.Address, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.addresses[customerID] {
		if a.ID() == id {
			return a, nil
		}
	}
	return shipDomain.Address{}, fmt.Errorf("address not found: %s", id)
}

func (s *adminCustomersE2EShippingStorage) Save(ctx context.Context, addr shipDomain.Address) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.addresses[addr.CustomerID()]
	for i, existing := range list {
		if existing.ID() == addr.ID() {
			list[i] = addr
			s.addresses[addr.CustomerID()] = list
			return nil
		}
	}
	s.addresses[addr.CustomerID()] = append(list, addr)
	return nil
}

func (s *adminCustomersE2EShippingStorage) Delete(ctx context.Context, customerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated []shipDomain.Address
	for _, a := range s.addresses[customerID] {
		if a.ID() != id {
			updated = append(updated, a)
		}
	}
	s.addresses[customerID] = updated
	return nil
}

func (s *adminCustomersE2EShippingStorage) ClearDefault(ctx context.Context, customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.addresses[customerID] {
		if a.IsDefault() {
			updated, _ := shipDomain.NewAddress(a.ID(), a.CustomerID(), a.Name(), a.Street1(), a.Street2(), a.City(), a.Zip(), a.Country(), false, a.CreatedAt())
			s.addresses[customerID][i] = updated
		}
	}
	return nil
}

func (s *adminCustomersE2EShippingStorage) MarkDefault(ctx context.Context, customerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, a := range s.addresses[customerID] {
		updated, _ := shipDomain.NewAddress(a.ID(), a.CustomerID(), a.Name(), a.Street1(), a.Street2(), a.City(), a.Zip(), a.Country(), a.ID() == id, a.CreatedAt())
		s.addresses[customerID][i] = updated
	}
	return nil
}

func (s *adminCustomersE2EShippingStorage) Count(ctx context.Context, customerID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.addresses[customerID]), nil
}

// adminCustomersE2ECheckoutService implements both checkoutCommands and checkoutQueries for E2E tests.
type adminCustomersE2ECheckoutService struct {
	mu         sync.Mutex
	summaries  map[string][]checkoutQuery.OrderSummary
	orderViews map[string]checkoutQuery.OrderView
	stats      []checkoutQuery.CustomerOrderStat
}

func newAdminCustomersE2ECheckoutService() *adminCustomersE2ECheckoutService {
	return &adminCustomersE2ECheckoutService{
		summaries:  make(map[string][]checkoutQuery.OrderSummary),
		orderViews: make(map[string]checkoutQuery.OrderView),
	}
}

func (c *adminCustomersE2ECheckoutService) AddOrder(
	customerID string,
	summary checkoutQuery.OrderSummary,
	view checkoutQuery.OrderView,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summaries[customerID] = append(c.summaries[customerID], summary)
	c.orderViews[summary.ID()] = view
}

func (c *adminCustomersE2ECheckoutService) SetStats(stats []checkoutQuery.CustomerOrderStat) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats = stats
}

func (c *adminCustomersE2ECheckoutService) Place(
	ctx context.Context,
	sessID, customerID, cardNumber string,
	shipTo checkoutDomain.Address,
	shipMethod checkoutDomain.ShippingMethod,
	payMethod checkoutDomain.PaymentMethod,
	discount promodomain.Discount,
) (checkoutDomain.Order, error) {
	return checkoutDomain.Order{}, nil
}

func (c *adminCustomersE2ECheckoutService) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) AdminCancel(ctx context.Context, orderID string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) MarkPaid(ctx context.Context, orderID string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) MarkPaymentFailed(ctx context.Context, orderID, reason string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return nil, nil
}

func (c *adminCustomersE2ECheckoutService) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	return checkoutDomain.ShippingMethod{}, nil
}

func (c *adminCustomersE2ECheckoutService) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	return nil
}

func (c *adminCustomersE2ECheckoutService) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if v, ok := c.orderViews[id]; ok {
		return v, nil
	}
	return checkoutQuery.OrderView{}, nil
}

func (c *adminCustomersE2ECheckoutService) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]checkoutQuery.OrderSummary(nil), c.summaries[customerID]...), nil
}

func (c *adminCustomersE2ECheckoutService) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var all []checkoutQuery.OrderSummary
	for _, list := range c.summaries {
		all = append(all, list...)
	}
	return all, nil
}

func (c *adminCustomersE2ECheckoutService) ListCustomerOrderStats(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]checkoutQuery.CustomerOrderStat(nil), c.stats...), nil
}

func (c *adminCustomersE2ECheckoutService) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return true, nil
}

func (c *adminCustomersE2ECheckoutService) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}

// adminCustomersE2EMailer records sent messages.
type adminCustomersE2EMailer struct {
	mu       sync.Mutex
	messages []mailer.Message
}

func (m *adminCustomersE2EMailer) Send(ctx context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *adminCustomersE2EMailer) Messages() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mailer.Message(nil), m.messages...)
}

// adminCustomersE2EBuyerChecker satisfies reviewsApp.VerifiedBuyerChecker.
type adminCustomersE2EBuyerChecker struct{}

func (b *adminCustomersE2EBuyerChecker) HasPurchased(ctx context.Context, customerID, productID string) (bool, error) {
	return true, nil
}

func setupAdminCustomersE2E(t *testing.T) (*mux.Router, *adminCustomersE2EMailer) {
	t.Helper()
	setupTestEnvironment(t)

	ctx := context.Background()

	// 1. Auth context: register users
	authStorage := authAdapter.NewInMemoryAuthStorage()
	sessionStorage := authAdapter.NewInMemorySessionStorage()
	resetStorage := authAdapter.NewInMemoryPasswordResetStorage()
	authSrv := authApp.NewAuth(authStorage, sessionStorage.CustomerScope(), resetStorage)

	if err := authSrv.CreateNewCustomer(ctx, "alice@example.com", "Password123!"); err != nil {
		t.Fatalf("failed to create customer alice: %v", err)
	}
	if err := authSrv.CreateNewCustomer(ctx, "charlie-reg@example.com", "Password123!"); err != nil {
		t.Fatalf("failed to create customer charlie: %v", err)
	}

	// 2. Shipping info context: saved addresses
	shipStorage := newAdminCustomersE2EShippingStorage()
	shipSrv := shippingApp.NewService(shipStorage, func() string { return "addr-alice-1" })
	if err := shipSrv.Add(ctx, "alice@example.com", "Alice Smith", "123 Main St", "Suite 400", "New York", "10001", "USA"); err != nil {
		t.Fatalf("failed to add address for alice: %v", err)
	}

	// 3. Reviews context: submitted review
	revStorage := reviewsAdapter.NewInMemory()
	revSrv := reviewsApp.NewService(revStorage, &adminCustomersE2EBuyerChecker{})
	aliceReview, err := reviewsDomain.NewReview(
		"rev-1",
		"prod-leather-jacket",
		"alice@example.com",
		"Incredible quality and fits perfectly! Highly recommended.",
		5,
		time.Now().UTC().Add(-24*time.Hour),
		reviewsDomain.StatusApproved,
	)
	if err != nil {
		t.Fatalf("failed to construct review: %v", err)
	}
	if err := revStorage.Insert(ctx, aliceReview); err != nil {
		t.Fatalf("failed to insert review: %v", err)
	}

	// 4. Wishlist context: bookmark
	wishStorage := wishlistAdapter.NewInMemory()
	wishSrv := wishlistApp.NewService(wishStorage)
	if err := wishStorage.Add(ctx, "alice@example.com", "var-jacket-brown-m", time.Now().UTC().Add(-48*time.Hour)); err != nil {
		t.Fatalf("failed to add wishlist item: %v", err)
	}

	// 5. Checkout context: orders and customer order stats
	checkoutSrv := newAdminCustomersE2ECheckoutService()
	aliceOrderTime := time.Now().UTC().Add(-2 * time.Hour)
	bobOrderTime := time.Now().UTC().Add(-1 * time.Hour)

	aliceSummary := checkoutQuery.NewOrderSummary(
		"alice@example.com",
		"ord-1001",
		checkoutDomain.StatusPaid,
		aliceOrderTime,
		2,
		19999,
		"USD",
	)
	checkoutSrv.AddOrder("alice@example.com", aliceSummary, checkoutQuery.OrderView{})

	bobSummary := checkoutQuery.NewOrderSummary(
		"bob-guest@example.com",
		"ord-1002",
		checkoutDomain.StatusPending,
		bobOrderTime,
		1,
		4999,
		"USD",
	)
	checkoutSrv.AddOrder("bob-guest@example.com", bobSummary, checkoutQuery.OrderView{})

	checkoutSrv.SetStats([]checkoutQuery.CustomerOrderStat{
		checkoutQuery.NewCustomerOrderStat("alice@example.com", "Alice Smith", 1, 19999, "USD", aliceOrderTime),
		checkoutQuery.NewCustomerOrderStat("bob-guest@example.com", "Bob Guestman", 1, 4999, "USD", bobOrderTime),
	})

	// 6. Mailer
	mailMock := &adminCustomersE2EMailer{}

	// 7. Admin auth
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	// 8. Bounded context wiring
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)

	bc := New(
		logger,
		nil, // cartSrv
		nil, // catalogSrv
		authSrv,
		adminAuth,
		checkoutSrv,
		checkoutSrv,
		nil, // fulfillmentSrv
		nil, // repricingSrv
		shipSrv,
		revSrv,
		wishSrv,
		nil, // promoSrv
		nil, // searchSrv
		nil, // storeSrv
		nil, // imageStore
		"",  // uploadsDir
		[]byte("test-secret-12345678901234567890"),
		false, // cookieSecure
		false, // csrfEnabled
		mailMock,
		"http://localhost:8080",
		rates,
		"",
		nil, // paymentsSrv
		nil, // limiter
		"",  // trustedProxies
		false,
	)

	router := mux.NewRouter()
	bc.(*boundedContext).MuxRegister(router)
	return router, mailMock
}

func TestAdminCustomers_E2E(t *testing.T) {
	router, mailMock := setupAdminCustomersE2E(t)

	t.Run("auth required for all customer endpoints", func(t *testing.T) {
		endpoints := []struct {
			method string
			url    string
		}{
			{http.MethodGet, "/admin/customers"},
			{http.MethodGet, "/admin/customers/alice@example.com"},
			{http.MethodPost, "/admin/customers/alice@example.com/reset-password"},
		}

		for _, ep := range endpoints {
			req := httptest.NewRequest(ep.method, ep.url, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Errorf("%s %s: expected 303 redirect to login, got %d", ep.method, ep.url, rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "/admin/login" {
				t.Errorf("%s %s: expected redirect to /admin/login, got %s", ep.method, ep.url, loc)
			}
		}
	})

	t.Run("customer listing shows registered and guest accounts with correct badges and stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /admin/customers, got %d", rec.Code)
		}

		body := rec.Body.String()

		// Verify all 3 customers appear in the list
		if !strings.Contains(body, "alice@example.com") {
			t.Errorf("expected listing to contain alice@example.com")
		}
		if !strings.Contains(body, "bob-guest@example.com") {
			t.Errorf("expected listing to contain bob-guest@example.com")
		}
		if !strings.Contains(body, "charlie-reg@example.com") {
			t.Errorf("expected listing to contain charlie-reg@example.com")
		}

		// Verify badges
		if !strings.Contains(body, `<span class="badge badge--success">Registered</span>`) {
			t.Errorf("expected listing to contain Registered badge")
		}
		if !strings.Contains(body, `<span class="badge badge--muted">Guest</span>`) {
			t.Errorf("expected listing to contain Guest badge")
		}

		// Verify shipping names
		if !strings.Contains(body, "Alice Smith") {
			t.Errorf("expected listing to display Alice Smith")
		}
		if !strings.Contains(body, "Bob Guestman") {
			t.Errorf("expected listing to display Bob Guestman")
		}

		// Verify totals rendered
		if !strings.Contains(body, "199.99 USD") {
			t.Errorf("expected listing to display 199.99 USD for alice")
		}
		if !strings.Contains(body, "49.99 USD") {
			t.Errorf("expected listing to display 49.99 USD for bob")
		}
	})

	t.Run("search customers by shipping name and email", func(t *testing.T) {
		// 1. Search by shipping name "Guestman"
		reqName := httptest.NewRequest(http.MethodGet, "/admin/customers?q=Guestman", nil)
		setAdminSession(t, reqName)
		recName := httptest.NewRecorder()
		router.ServeHTTP(recName, reqName)

		if recName.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recName.Code)
		}
		bodyName := recName.Body.String()
		if !strings.Contains(bodyName, "bob-guest@example.com") {
			t.Errorf("expected search for 'Guestman' to match bob-guest@example.com")
		}
		if !strings.Contains(bodyName, "Bob Guestman") {
			t.Errorf("expected search for 'Guestman' to display Bob Guestman")
		}
		if strings.Contains(bodyName, "alice@example.com") {
			t.Errorf("expected search for 'Guestman' to exclude alice@example.com")
		}
		if strings.Contains(bodyName, "charlie-reg@example.com") {
			t.Errorf("expected search for 'Guestman' to exclude charlie-reg@example.com")
		}

		// 2. Search by email "alice"
		reqEmail := httptest.NewRequest(http.MethodGet, "/admin/customers?q=alice", nil)
		setAdminSession(t, reqEmail)
		recEmail := httptest.NewRecorder()
		router.ServeHTTP(recEmail, reqEmail)

		if recEmail.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recEmail.Code)
		}
		bodyEmail := recEmail.Body.String()
		if !strings.Contains(bodyEmail, "alice@example.com") {
			t.Errorf("expected search for 'alice' to match alice@example.com")
		}
		if strings.Contains(bodyEmail, "bob-guest@example.com") {
			t.Errorf("expected search for 'alice' to exclude bob-guest@example.com")
		}
		if strings.Contains(bodyEmail, "charlie-reg@example.com") {
			t.Errorf("expected search for 'alice' to exclude charlie-reg@example.com")
		}
	})

	t.Run("status filtering: registered, guest, and all", func(t *testing.T) {
		// 1. Filter: registered
		reqReg := httptest.NewRequest(http.MethodGet, "/admin/customers?filter=registered", nil)
		setAdminSession(t, reqReg)
		recReg := httptest.NewRecorder()
		router.ServeHTTP(recReg, reqReg)

		if recReg.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recReg.Code)
		}
		bodyReg := recReg.Body.String()
		if !strings.Contains(bodyReg, "alice@example.com") {
			t.Errorf("expected registered filter to include alice@example.com")
		}
		if !strings.Contains(bodyReg, "charlie-reg@example.com") {
			t.Errorf("expected registered filter to include charlie-reg@example.com")
		}
		if strings.Contains(bodyReg, "bob-guest@example.com") {
			t.Errorf("expected registered filter to exclude bob-guest@example.com")
		}

		// 2. Filter: guest
		reqGuest := httptest.NewRequest(http.MethodGet, "/admin/customers?filter=guest", nil)
		setAdminSession(t, reqGuest)
		recGuest := httptest.NewRecorder()
		router.ServeHTTP(recGuest, reqGuest)

		if recGuest.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recGuest.Code)
		}
		bodyGuest := recGuest.Body.String()
		if !strings.Contains(bodyGuest, "bob-guest@example.com") {
			t.Errorf("expected guest filter to include bob-guest@example.com")
		}
		if strings.Contains(bodyGuest, "alice@example.com") {
			t.Errorf("expected guest filter to exclude alice@example.com")
		}
		if strings.Contains(bodyGuest, "charlie-reg@example.com") {
			t.Errorf("expected guest filter to exclude charlie-reg@example.com")
		}

		// 3. Filter: all
		reqAll := httptest.NewRequest(http.MethodGet, "/admin/customers?filter=all", nil)
		setAdminSession(t, reqAll)
		recAll := httptest.NewRecorder()
		router.ServeHTTP(recAll, reqAll)

		if recAll.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", recAll.Code)
		}
		bodyAll := recAll.Body.String()
		if !strings.Contains(bodyAll, "alice@example.com") ||
			!strings.Contains(bodyAll, "bob-guest@example.com") ||
			!strings.Contains(bodyAll, "charlie-reg@example.com") {
			t.Errorf("expected all filter to include alice, bob, and charlie")
		}
	})

	t.Run("customer detail page renders orders, addresses, reviews, wishlist and reset button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/alice@example.com", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /admin/customers/alice@example.com, got %d", rec.Code)
		}

		body := rec.Body.String()

		// 1. Header & Identity
		if !strings.Contains(body, "customer <span class=\"order__id\">alice@example.com</span>") {
			t.Errorf("expected heading with alice@example.com")
		}
		if !strings.Contains(body, `<span class="badge badge--success">Registered Account</span>`) {
			t.Errorf("expected Registered Account badge")
		}
		if !strings.Contains(body, "<strong>Alice Smith</strong>") {
			t.Errorf("expected Alice Smith name in subtitle")
		}

		// 2. Stats
		if !strings.Contains(body, "199.99 USD") {
			t.Errorf("expected 199.99 USD in stats")
		}
		if !strings.Contains(body, "Total Orders") {
			t.Errorf("expected Total Orders stat label")
		}
		if !strings.Contains(body, "Average Order Value") {
			t.Errorf("expected Average Order Value stat label")
		}

		// 3. Orders Section
		if !strings.Contains(body, "ord-1001") {
			t.Errorf("expected order ord-1001 to render in orders section")
		}
		if !strings.Contains(body, "paid") {
			t.Errorf("expected order status 'paid' to render")
		}

		// 4. Shipping Addresses Section
		if !strings.Contains(body, "123 Main St") {
			t.Errorf("expected street '123 Main St' to render in addresses")
		}
		if !strings.Contains(body, "Suite 400") {
			t.Errorf("expected suite 'Suite 400' to render in addresses")
		}
		if !strings.Contains(body, "New York, 10001") {
			t.Errorf("expected 'New York, 10001' to render in addresses")
		}
		if !strings.Contains(body, "USA") {
			t.Errorf("expected country 'USA' to render in addresses")
		}
		if !strings.Contains(body, `<span class="badge badge--primary" style="margin-bottom: 6px;">default</span>`) {
			t.Errorf("expected default badge on address card")
		}

		// 5. Reviews Section
		if !strings.Contains(body, "prod-leather-jacket") {
			t.Errorf("expected product 'prod-leather-jacket' to render in reviews")
		}
		if !strings.Contains(body, "5 / 5") {
			t.Errorf("expected rating '5 / 5' to render in reviews")
		}
		if !strings.Contains(body, "Incredible quality and fits perfectly! Highly recommended.") {
			t.Errorf("expected review comment to render")
		}
		if !strings.Contains(body, "approved") {
			t.Errorf("expected review status 'approved' to render")
		}

		// 6. Wishlist Section
		if !strings.Contains(body, "var-jacket-brown-m") {
			t.Errorf("expected variant 'var-jacket-brown-m' to render in wishlist")
		}

		// 7. Password Reset Form for Registered Account
		if !strings.Contains(body, `action="/admin/customers/alice@example.com/reset-password"`) {
			t.Errorf("expected password reset form action")
		}
		if !strings.Contains(body, "Send Password Reset Email") {
			t.Errorf("expected 'Send Password Reset Email' button")
		}
	})

	t.Run("customer detail page for guest renders guest badge and empty states", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/bob-guest@example.com", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK from /admin/customers/bob-guest@example.com, got %d", rec.Code)
		}

		body := rec.Body.String()

		// Verify Guest badge
		if !strings.Contains(body, `<span class="badge badge--muted">Guest Purchaser</span>`) {
			t.Errorf("expected Guest Purchaser badge")
		}
		if !strings.Contains(body, "<strong>Bob Guestman</strong>") {
			t.Errorf("expected Bob Guestman display name")
		}

		// Verify order rendered
		if !strings.Contains(body, "ord-1002") {
			t.Errorf("expected guest order ord-1002 to render")
		}

		// Verify empty states for addresses, reviews, wishlist
		if !strings.Contains(body, "no saved addresses.") {
			t.Errorf("expected 'no saved addresses.' empty state")
		}
		if !strings.Contains(body, "no reviews submitted.") {
			t.Errorf("expected 'no reviews submitted.' empty state")
		}
		if !strings.Contains(body, "wishlist is empty.") {
			t.Errorf("expected 'wishlist is empty.' empty state")
		}

		// Verify NO password reset button is rendered for guest customer
		if strings.Contains(body, "Send Password Reset Email") {
			t.Errorf("expected 'Send Password Reset Email' button to NOT be rendered for guest")
		}
	})

	t.Run("trigger password reset for registered customer sends email and renders flash message", func(t *testing.T) {
		initialCount := len(mailMock.Messages())

		// Step 1: POST to reset password
		req := httptest.NewRequest(http.MethodPost, "/admin/customers/alice@example.com/reset-password", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/customers/alice@example.com" {
			t.Fatalf("expected redirect to /admin/customers/alice@example.com, got %s", loc)
		}

		// Step 2: Verify mailer received reset token message
		messages := mailMock.Messages()
		if len(messages) != initialCount+1 {
			t.Fatalf("expected mailer to receive 1 new email, got %d (was %d)", len(messages), initialCount)
		}

		lastMsg := messages[len(messages)-1]
		if lastMsg.To != "alice@example.com" {
			t.Errorf("expected email to be sent to alice@example.com, got %s", lastMsg.To)
		}
		if lastMsg.Kind != mailer.KindPasswordReset {
			t.Errorf("expected KindPasswordReset, got %s", lastMsg.Kind)
		}
		if !strings.Contains(lastMsg.TextBody, "/auth/reset?token=") {
			t.Errorf("expected reset email text body to contain reset link with token, got: %s", lastMsg.TextBody)
		}
		if !strings.Contains(lastMsg.HTMLBody, "/auth/reset?token=") {
			t.Errorf("expected reset email html body to contain reset link with token")
		}

		// Step 3: Follow redirect and verify flash message is rendered
		reqFollow := httptest.NewRequest(http.MethodGet, "/admin/customers/alice@example.com", nil)
		setAdminSession(t, reqFollow)
		for _, c := range rec.Result().Cookies() {
			reqFollow.AddCookie(c)
		}
		recFollow := httptest.NewRecorder()
		router.ServeHTTP(recFollow, reqFollow)

		if recFollow.Code != http.StatusOK {
			t.Fatalf("expected 200 OK after following redirect, got %d", recFollow.Code)
		}
		followBody := recFollow.Body.String()
		if !strings.Contains(followBody, "Password reset email sent to alice@example.com") {
			t.Errorf("expected flash message 'Password reset email sent to alice@example.com' in response HTML, got body: %s", followBody)
		}
		if !strings.Contains(followBody, "flash flash--info") {
			t.Errorf("expected flash--info class in rendered flash")
		}
	})

	t.Run("trigger password reset for guest customer is rejected and renders error flash", func(t *testing.T) {
		initialCount := len(mailMock.Messages())

		// Step 1: POST to reset password for guest
		req := httptest.NewRequest(http.MethodPost, "/admin/customers/bob-guest@example.com/reset-password", nil)
		setAdminSession(t, req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/customers/bob-guest@example.com" {
			t.Fatalf("expected redirect to /admin/customers/bob-guest@example.com, got %s", loc)
		}

		// Step 2: Verify mailer received NO emails
		messages := mailMock.Messages()
		if len(messages) != initialCount {
			t.Fatalf("expected no email sent for guest password reset, but sent %d emails", len(messages)-initialCount)
		}

		// Step 3: Follow redirect and verify error flash message is rendered
		reqFollow := httptest.NewRequest(http.MethodGet, "/admin/customers/bob-guest@example.com", nil)
		setAdminSession(t, reqFollow)
		for _, c := range rec.Result().Cookies() {
			reqFollow.AddCookie(c)
		}
		recFollow := httptest.NewRecorder()
		router.ServeHTTP(recFollow, reqFollow)

		if recFollow.Code != http.StatusOK {
			t.Fatalf("expected 200 OK after following redirect, got %d", recFollow.Code)
		}
		followBody := recFollow.Body.String()
		if !strings.Contains(followBody, "Cannot reset password for guest customer") {
			t.Errorf("expected error flash 'Cannot reset password for guest customer' in response HTML, got body: %s", followBody)
		}
		if !strings.Contains(followBody, "flash flash--error") {
			t.Errorf("expected flash--error class in rendered flash")
		}
	})
}
