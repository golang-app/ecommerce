package layout_test

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

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type e2eDemoAdminAuthService struct {
	passwordChanged bool
	mustChange      bool
}

func (f *e2eDemoAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", email, time.Now().Add(time.Hour)), nil
}
func (f *e2eDemoAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f *e2eDemoAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f *e2eDemoAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	f.passwordChanged = true
	return nil
}
func (f *e2eDemoAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return f.mustChange, nil
}
func (f *e2eDemoAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

type e2eDemoCartService struct {
	cart *cartDomain.Cart
}

func (c *e2eDemoCartService) AddToCart(ctx context.Context, sessID string, productID string, qty int) error {
	return nil
}

func (c *e2eDemoCartService) Get(ctx context.Context, sessID string) (*cartDomain.Cart, error) {
	if c.cart != nil {
		return c.cart, nil
	}
	return nil, cartDomain.ErrCartNotFound
}

type e2eMockCheckoutCommands struct {
	methods []checkoutDomain.ShippingMethod
}

func (m *e2eMockCheckoutCommands) Place(ctx context.Context, sessID, customerID, cardNumber string, shipTo checkoutDomain.Address, shipMethod checkoutDomain.ShippingMethod, payMethod checkoutDomain.PaymentMethod, discount promodomain.Discount) (checkoutDomain.Order, error) {
	return checkoutDomain.Order{}, nil
}
func (m *e2eMockCheckoutCommands) Cancel(ctx context.Context, orderID, customerID string) error {
	return nil
}
func (m *e2eMockCheckoutCommands) AdminCancel(ctx context.Context, orderID string) error { return nil }
func (m *e2eMockCheckoutCommands) MarkPaid(ctx context.Context, orderID string) error          { return nil }
func (m *e2eMockCheckoutCommands) MarkPaymentFailed(ctx context.Context, orderID string, reason string) error {
	return nil
}
func (m *e2eMockCheckoutCommands) ListShippingMethods(ctx context.Context) ([]checkoutDomain.ShippingMethod, error) {
	return m.methods, nil
}
func (m *e2eMockCheckoutCommands) FindShippingMethod(ctx context.Context, code string) (checkoutDomain.ShippingMethod, error) {
	for _, sm := range m.methods {
		if sm.Code() == code {
			return sm, nil
		}
	}
	return checkoutDomain.ShippingMethod{}, nil
}
func (m *e2eMockCheckoutCommands) UpdateShippingMethod(ctx context.Context, code string, enabled bool, label string, cost int64, carrier string) error {
	return nil
}
func (m *e2eMockCheckoutCommands) UpdateTracking(ctx context.Context, orderID, carrier, trackingCode string) error {
	return nil
}

func TestDemoMode_FullLifecycle_E2E(t *testing.T) {
	is := is.New(t)

	if origWd, err := os.Getwd(); err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	// ==========================================
	// 1. RUN WITH DEMO MODE ENABLED (APP_DEMO=true)
	// ==========================================
	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage)
	adminAuth := &e2eDemoAdminAuthService{mustChange: true}

	cart := cartDomain.NewCart(cartDomain.NewUser(""))
	_ = cart.Add(cartDomain.NewProduct("prod-demo", "Demo Product", 5000, cartDomain.MustNewCurrency("USD")), 1)
	cartSrv := &e2eDemoCartService{cart: cart}

	methods := []checkoutDomain.ShippingMethod{
		checkoutDomain.NewShippingMethod("flat", "Flat rate", 500, true, "Standard Post", true),
	}
	mockCmds := &e2eMockCheckoutCommands{methods: methods}

	bcDemo := layout.New(
		logger,
		cartSrv, catalogSrv, nil, adminAuth, mockCmds, guardsCheckoutQry{}, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		true, // demoMode = true
	)

	routerDemo := mux.NewRouter()
	if mr, ok := bcDemo.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(routerDemo)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	// Step 1a: Storefront home page displays top demo banner
	reqHome := httptest.NewRequest(http.MethodGet, "/", nil)
	recHome := httptest.NewRecorder()
	routerDemo.ServeHTTP(recHome, reqHome)
	is.Equal(recHome.Code, http.StatusOK)
	is.True(strings.Contains(recHome.Body.String(), "demo-banner"))
	is.True(strings.Contains(recHome.Body.String(), "DEMO MODE"))
	is.True(strings.Contains(recHome.Body.String(), "interactive demo store"))

	// Step 1b: Customer login page displays demo credentials box
	reqCustLogin := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	recCustLogin := httptest.NewRecorder()
	routerDemo.ServeHTTP(recCustLogin, reqCustLogin)
	is.Equal(recCustLogin.Code, http.StatusOK)
	is.True(strings.Contains(recCustLogin.Body.String(), "demo-credentials-box"))
	is.True(strings.Contains(recCustLogin.Body.String(), "customer@example.com"))

	// Step 1c: Admin login page displays admin demo banner & credentials box
	reqAdminLogin := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	recAdminLogin := httptest.NewRecorder()
	routerDemo.ServeHTTP(recAdminLogin, reqAdminLogin)
	is.Equal(recAdminLogin.Code, http.StatusOK)
	is.True(strings.Contains(recAdminLogin.Body.String(), "admin-demo-banner"))
	is.True(strings.Contains(recAdminLogin.Body.String(), "admin-demo-credentials-box"))
	is.True(strings.Contains(recAdminLogin.Body.String(), "admin@example.com"))

	// Step 1d: Admin logs in -> bypasses forced password change -> lands on /admin dashboard
	formLogin := url.Values{"email": {"admin@example.com"}, "password": {"Admin123!"}}
	reqPostLogin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(formLogin.Encode()))
	reqPostLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostLogin := httptest.NewRecorder()
	routerDemo.ServeHTTP(recPostLogin, reqPostLogin)
	is.Equal(recPostLogin.Code, http.StatusSeeOther)
	is.Equal(recPostLogin.Header().Get("Location"), "/admin")

	adminCookies := recPostLogin.Result().Cookies()

	// Step 1e: Access /admin dashboard -> admin-demo-banner is present
	reqDash := httptest.NewRequest(http.MethodGet, "/admin", nil)
	applyCookies(reqDash, adminCookies)
	recDash := httptest.NewRecorder()
	routerDemo.ServeHTTP(recDash, reqDash)
	is.Equal(recDash.Code, http.StatusOK)
	is.True(strings.Contains(recDash.Body.String(), "admin-demo-banner"))

	// Step 1f: Mutation guards -> Category deletion is blocked
	ctx := context.Background()
	_ = catalogSrv.CreateCategory(ctx, "Demo Cat", "demo-cat", "")
	reqDelCat := httptest.NewRequest(http.MethodPost, "/admin/categories/demo-cat/delete", nil)
	applyCookies(reqDelCat, adminCookies)
	recDelCat := httptest.NewRecorder()
	routerDemo.ServeHTTP(recDelCat, reqDelCat)
	is.Equal(recDelCat.Code, http.StatusSeeOther)
	is.Equal(recDelCat.Header().Get("Location"), "/admin/categories")

	cats, err := catalogSrv.Categories(ctx)
	is.NoErr(err)
	is.Equal(len(cats), 1)

	// Step 1g: Checkout page displays demo notice and simulated email hints
	reqCheckout := httptest.NewRequest(http.MethodGet, "/checkout", nil)
	reqCheckout.AddCookie(&http.Cookie{Name: "cart_id", Value: "cart-demo"})
	recCheckout := httptest.NewRecorder()
	routerDemo.ServeHTTP(recCheckout, reqCheckout)
	is.Equal(recCheckout.Code, http.StatusOK)
	is.True(strings.Contains(recCheckout.Body.String(), "demo-checkout-notice"))
	is.True(strings.Contains(recCheckout.Body.String(), "No real payment is processed"))
	is.True(strings.Contains(recCheckout.Body.String(), "Order confirmation emails are simulated"))

	// ==========================================
	// 2. RUN WITH DEMO MODE DISABLED (APP_DEMO=false)
	// ==========================================
	pcStorageNoDemo := pcadapter.NewInMemory()
	catalogSrvNoDemo := pcapp.NewProductService(pcStorageNoDemo)
	adminAuthNoDemo := &e2eDemoAdminAuthService{}

	bcNoDemo := layout.New(
		logger,
		cartSrv, catalogSrvNoDemo, nil, adminAuthNoDemo, mockCmds, guardsCheckoutQry{}, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		false, // demoMode = false
	)

	routerNoDemo := mux.NewRouter()
	if mr, ok := bcNoDemo.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(routerNoDemo)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	// Step 2a: Storefront home page does NOT display demo banner
	recHome2 := httptest.NewRecorder()
	routerNoDemo.ServeHTTP(recHome2, reqHome)
	is.Equal(recHome2.Code, http.StatusOK)
	is.True(!strings.Contains(recHome2.Body.String(), "demo-banner"))

	// Step 2b: Customer login page does NOT display demo credentials box
	recCustLogin2 := httptest.NewRecorder()
	routerNoDemo.ServeHTTP(recCustLogin2, reqCustLogin)
	is.Equal(recCustLogin2.Code, http.StatusOK)
	is.True(!strings.Contains(recCustLogin2.Body.String(), "demo-credentials-box"))

	// Step 2c: Admin login page does NOT display admin demo banner or box
	recAdminLogin2 := httptest.NewRecorder()
	routerNoDemo.ServeHTTP(recAdminLogin2, reqAdminLogin)
	is.Equal(recAdminLogin2.Code, http.StatusOK)
	is.True(!strings.Contains(recAdminLogin2.Body.String(), "admin-demo-banner"))
	is.True(!strings.Contains(recAdminLogin2.Body.String(), "admin-demo-credentials-box"))

	// Step 2d: Category deletion is permitted when demo mode is false
	_ = catalogSrvNoDemo.CreateCategory(ctx, "Cat Normal", "cat-normal", "")
	reqPostLogin2 := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(formLogin.Encode()))
	reqPostLogin2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recPostLogin2 := httptest.NewRecorder()
	routerNoDemo.ServeHTTP(recPostLogin2, reqPostLogin2)
	adminCookies2 := recPostLogin2.Result().Cookies()

	reqDelCat2 := httptest.NewRequest(http.MethodPost, "/admin/categories/cat-normal/delete", nil)
	applyCookies(reqDelCat2, adminCookies2)
	recDelCat2 := httptest.NewRecorder()
	routerNoDemo.ServeHTTP(recDelCat2, reqDelCat2)
	is.Equal(recDelCat2.Code, http.StatusSeeOther)

	catsNormal, err := catalogSrvNoDemo.Categories(ctx)
	is.NoErr(err)
	is.Equal(len(catsNormal), 0)
}
