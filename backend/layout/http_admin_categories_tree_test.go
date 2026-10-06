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
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type fakeAdminAuthService struct{}

func (f fakeAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f fakeAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f fakeAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f fakeAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f fakeAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f fakeAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

type treeTestApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
}

func newTreeTestApp(t *testing.T) *treeTestApp {
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
	adminAuth := fakeAdminAuthService{}

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, adminAuth, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return &treeTestApp{
		router:     router,
		catalogSrv: &catalogSrv,
	}
}

func loginAsAdmin(t *testing.T, app *treeTestApp) *http.Cookie {
	t.Helper()
	form := url.Values{"email": {"admin@example.com"}, "password": {"secret"}}
	req := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == "ecommerce_admin" {
			return c
		}
	}
	t.Fatalf("failed to obtain admin session cookie")
	return nil
}

func TestAdminCategories_TreeDisplayAndSafeDelete(t *testing.T) {
	app := newTreeTestApp(t)
	adminCookie := loginAsAdmin(t, app)

	// Create root category "Apparel"
	form := url.Values{"name": {"Apparel"}, "slug": {"apparel"}, "parent_id": {""}}
	req := httptest.NewRequest("POST", "/admin/categories", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on create apparel, got %d", rec.Code)
	}

	// Create child category "Shoes" under Apparel
	form = url.Values{"name": {"Shoes"}, "slug": {"shoes"}, "parent_id": {"apparel"}}
	req = httptest.NewRequest("POST", "/admin/categories", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on create shoes, got %d", rec.Code)
	}

	// Verify GET /admin/categories displays tree visual indentation and parent dropdown option
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /admin/categories, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Apparel") {
		t.Errorf("expected GET /admin/categories body to contain 'Apparel'")
	}
	if !strings.Contains(body, "└── Shoes") && !strings.Contains(body, "Shoes") {
		t.Errorf("expected GET /admin/categories body to contain 'Shoes'")
	}
	if !strings.Contains(body, `<select id="c-parent" name="parent_id">`) && !strings.Contains(body, `name="parent_id"`) {
		t.Errorf("expected GET /admin/categories form to contain parent_id select")
	}

	// Attempt delete Apparel -> expect redirect with flash error
	req = httptest.NewRequest("POST", "/admin/categories/apparel/delete", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on delete apparel with children, got %d", rec.Code)
	}

	// Follow redirect to check flash message
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.AddCookie(adminCookie)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	body = rec.Body.String()
	if !strings.Contains(body, "Cannot delete category because it has subcategories") {
		t.Errorf("expected body to contain error flash 'Cannot delete category because it has subcategories', got: %s", body)
	}

	// Delete child category "Shoes"
	req = httptest.NewRequest("POST", "/admin/categories/shoes/delete", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on delete shoes, got %d", rec.Code)
	}

	// Delete root category "Apparel" after child removed
	req = httptest.NewRequest("POST", "/admin/categories/apparel/delete", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on delete apparel after child removed, got %d", rec.Code)
	}
}

func TestAdminEditCategory_ParentOptionsAndCyclePrevention(t *testing.T) {
	app := newTreeTestApp(t)
	adminCookie := loginAsAdmin(t, app)

	// Create Apparel -> Shoes -> Sneakers
	for _, cat := range []struct{ name, slug, parent string }{
		{"Apparel", "apparel", ""},
		{"Shoes", "shoes", "apparel"},
		{"Sneakers", "sneakers", "shoes"},
	} {
		form := url.Values{"name": {cat.name}, "slug": {cat.slug}, "parent_id": {cat.parent}}
		req := httptest.NewRequest("POST", "/admin/categories", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		app.router.ServeHTTP(rec, req)
	}

	// GET /admin/categories/shoes/edit
	req := httptest.NewRequest("GET", "/admin/categories/shoes/edit", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on edit shoes form, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="parent_id"`) {
		t.Errorf("expected edit category form to contain parent_id select")
	}

	// Attempt cycle: update Apparel with parent_id=sneakers
	form := url.Values{"name": {"Apparel"}, "slug": {"apparel"}, "parent_id": {"sneakers"}, "position": {"0"}}
	req = httptest.NewRequest("POST", "/admin/categories/apparel", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect on cyclic update, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "/admin/categories/apparel/edit") {
		t.Errorf("expected redirect to edit form /admin/categories/apparel/edit, got %s", rec.Header().Get("Location"))
	}

	// Follow redirect to check flash message
	req = httptest.NewRequest("GET", "/admin/categories/apparel/edit", nil)
	req.AddCookie(adminCookie)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	body = rec.Body.String()
	if !strings.Contains(body, "cyclic") {
		t.Errorf("expected flash message to contain 'cyclic', got: %s", body)
	}
}
