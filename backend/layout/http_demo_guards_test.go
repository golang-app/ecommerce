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
	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type guardsCheckoutQry struct{}

func (g guardsCheckoutQry) Find(ctx context.Context, id string) (checkoutQuery.OrderView, error) {
	return checkoutQuery.OrderView{}, nil
}
func (g guardsCheckoutQry) ListByCustomer(ctx context.Context, customerID string) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}
func (g guardsCheckoutQry) ListAll(ctx context.Context) ([]checkoutQuery.OrderSummary, error) {
	return nil, nil
}
func (g guardsCheckoutQry) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	return false, nil
}
func (g guardsCheckoutQry) TodaysSales(ctx context.Context) (map[string]checkoutQuery.DailySalesRow, error) {
	return nil, nil
}
func (g guardsCheckoutQry) ListCustomerOrderStats(ctx context.Context) ([]checkoutQuery.CustomerOrderStat, error) {
	return nil, nil
}

type guardsAdminAuthService struct {
	passwordChanged bool
	mustChange      bool
}

func (f *guardsAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", email, time.Now().Add(time.Hour)), nil
}
func (f *guardsAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f *guardsAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f *guardsAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	f.passwordChanged = true
	return nil
}
func (f *guardsAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return f.mustChange, nil
}
func (f *guardsAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

type guardsTestApp struct {
	router     *mux.Router
	catalogSrv pcapp.ProductService
	adminAuth  *guardsAdminAuthService
}

func newGuardsTestApp(t *testing.T, demoMode bool) *guardsTestApp {
	t.Helper()

	if origWd, err := os.Getwd(); err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage)
	adminAuth := &guardsAdminAuthService{}

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, adminAuth, nil, guardsCheckoutQry{}, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		demoMode,
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return &guardsTestApp{
		router:     router,
		catalogSrv: catalogSrv,
		adminAuth:  adminAuth,
	}
}

func loginAdminHelper(t *testing.T, app *guardsTestApp) []*http.Cookie {
	t.Helper()
	form := url.Values{}
	form.Set("email", "admin@example.com")
	form.Set("password", "Admin123!")

	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	return rec.Result().Cookies()
}

func applyCookies(req *http.Request, cookieSets ...[]*http.Cookie) {
	m := make(map[string]*http.Cookie)
	for _, set := range cookieSets {
		for _, c := range set {
			m[c.Name] = c
		}
	}
	for _, c := range m {
		req.AddCookie(c)
	}
}

func TestDemoGuards_CategoryDeletionBlocked(t *testing.T) {
	is := is.New(t)
	app := newGuardsTestApp(t, true)
	ctx := context.Background()

	err := app.catalogSrv.CreateCategory(ctx, "Test Cat", "cat-1", "")
	is.NoErr(err)

	cookies := loginAdminHelper(t, app)

	req := httptest.NewRequest(http.MethodPost, "/admin/categories/cat-1/delete", nil)
	applyCookies(req, cookies)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/categories")

	// Verify category was NOT deleted
	cats, err := app.catalogSrv.Categories(ctx)
	is.NoErr(err)
	is.Equal(len(cats), 1)

	// Follow redirect and check flash message
	reqFollow := httptest.NewRequest(http.MethodGet, "/admin/categories", nil)
	applyCookies(reqFollow, cookies, rec.Result().Cookies())
	recFollow := httptest.NewRecorder()
	app.router.ServeHTTP(recFollow, reqFollow)
	is.True(strings.Contains(recFollow.Body.String(), "Category deletion is disabled in Demo Mode."))
}

func TestDemoGuards_ProductDeletionBlocked(t *testing.T) {
	is := is.New(t)
	app := newGuardsTestApp(t, true)
	ctx := context.Background()

	err := app.catalogSrv.Add(ctx, "prod-1", "Test Product", "Description", 1000, "USD", "")
	is.NoErr(err)

	cookies := loginAdminHelper(t, app)

	req := httptest.NewRequest(http.MethodPost, "/admin/products/prod-1/delete", nil)
	applyCookies(req, cookies)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/products")

	// Verify product was NOT deleted
	p, err := app.catalogSrv.Find(ctx, "prod-1")
	is.NoErr(err)
	is.Equal(string(p.ID()), "prod-1")

	// Follow redirect and check flash message
	reqFollow := httptest.NewRequest(http.MethodGet, "/admin/products", nil)
	applyCookies(reqFollow, cookies, rec.Result().Cookies())
	recFollow := httptest.NewRecorder()
	app.router.ServeHTTP(recFollow, reqFollow)
	is.True(strings.Contains(recFollow.Body.String(), "Product deletion is disabled in Demo Mode."))
}

func TestDemoGuards_AdminChangePasswordBlocked(t *testing.T) {
	is := is.New(t)
	app := newGuardsTestApp(t, true)
	app.adminAuth.mustChange = true

	cookies := loginAdminHelper(t, app)

	form := url.Values{}
	form.Set("current_password", "Admin123!")
	form.Set("new_password", "SuperSecret999!")
	form.Set("confirm_password", "SuperSecret999!")

	req := httptest.NewRequest(http.MethodPost, "/admin/change-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	applyCookies(req, cookies)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin")
	is.Equal(app.adminAuth.passwordChanged, false)

	// Follow redirect and check flash error
	reqFollow := httptest.NewRequest(http.MethodGet, "/admin", nil)
	applyCookies(reqFollow, cookies, rec.Result().Cookies())
	recFollow := httptest.NewRecorder()
	app.router.ServeHTTP(recFollow, reqFollow)
	is.True(strings.Contains(recFollow.Body.String(), "Password modification for demo admin is disabled in Demo Mode."))
}

func TestDemoGuards_CategoryDeletionAllowedWhenDemoModeOff(t *testing.T) {
	is := is.New(t)
	app := newGuardsTestApp(t, false)
	ctx := context.Background()

	err := app.catalogSrv.CreateCategory(ctx, "Test Cat", "cat-allow-1", "")
	is.NoErr(err)

	cookies := loginAdminHelper(t, app)

	req := httptest.NewRequest(http.MethodPost, "/admin/categories/cat-allow-1/delete", nil)
	applyCookies(req, cookies)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/categories")

	// Verify category was deleted
	cats, err := app.catalogSrv.Categories(ctx)
	is.NoErr(err)
	is.Equal(len(cats), 0)
}

func TestDemoGuards_ProductDeletionAllowedWhenDemoModeOff(t *testing.T) {
	is := is.New(t)
	app := newGuardsTestApp(t, false)
	ctx := context.Background()

	err := app.catalogSrv.Add(ctx, "prod-allow-1", "Test Product", "Description", 1000, "USD", "")
	is.NoErr(err)

	cookies := loginAdminHelper(t, app)

	req := httptest.NewRequest(http.MethodPost, "/admin/products/prod-allow-1/delete", nil)
	applyCookies(req, cookies)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/products")

	// Verify product was deleted
	_, err = app.catalogSrv.Find(ctx, "prod-allow-1")
	is.True(err != nil)
}
