package layout_test

import (
	"bytes"
	"context"
	"mime/multipart"
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

type categoryE2EAdminAuth struct{}

func (f categoryE2EAdminAuth) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f categoryE2EAdminAuth) Logout(ctx context.Context, token string) error { return nil }
func (f categoryE2EAdminAuth) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f categoryE2EAdminAuth) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f categoryE2EAdminAuth) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f categoryE2EAdminAuth) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

type categoryE2EApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
}

func newCategoryE2EApp(t *testing.T) *categoryE2EApp {
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
	adminAuth := categoryE2EAdminAuth{}

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

	return &categoryE2EApp{
		router:     router,
		catalogSrv: &catalogSrv,
	}
}

func loginAdminForE2E(t *testing.T, app *categoryE2EApp) *http.Cookie {
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

func TestCategoryTree_E2E(t *testing.T) {
	app := newCategoryE2EApp(t)

	// Step 2: Sign in as admin
	adminCookie := loginAdminForE2E(t, app)

	// Step 3: Create category hierarchy: Vehicles -> Cars -> Electric
	categoriesToCreate := []struct {
		name     string
		slug     string
		parentID string
	}{
		{name: "Vehicles", slug: "vehicles", parentID: ""},
		{name: "Cars", slug: "cars", parentID: "vehicles"},
		{name: "Electric", slug: "electric", parentID: "cars"},
	}

	for _, cat := range categoriesToCreate {
		form := url.Values{
			"name":      {cat.name},
			"slug":      {cat.slug},
			"parent_id": {cat.parentID},
		}
		req := httptest.NewRequest("POST", "/admin/categories", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(adminCookie)
		rec := httptest.NewRecorder()
		app.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected HTTP 303 SeeOther on creating category %s, got %d", cat.name, rec.Code)
		}
	}

	// Step 4: Create a product and assign it to Electric
	var prodBody bytes.Buffer
	writer := multipart.NewWriter(&prodBody)
	_ = writer.WriteField("id", "ev-01")
	_ = writer.WriteField("name", "CyberTruck")
	_ = writer.WriteField("description", "Electric truck")
	_ = writer.WriteField("price", "79999.00")
	_ = writer.WriteField("currency", "USD")
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/products", &prodBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected HTTP 303 SeeOther on creating product, got %d", rec.Code)
	}

	// Assign product to 'electric' category
	assignForm := url.Values{
		"category": {"electric"},
	}
	req = httptest.NewRequest("POST", "/admin/products/ev-01/categories", strings.NewReader(assignForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected HTTP 303 SeeOther on assigning category to product, got %d", rec.Code)
	}

	// Step 5: Verify admin category list displays all 3 with proper visual indentation
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on GET /admin/categories, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Vehicles") {
		t.Errorf("expected admin category list to contain 'Vehicles'")
	}
	if !strings.Contains(body, "└── Cars") {
		t.Errorf("expected admin category list to contain indented '└── Cars', body:\n%s", body)
	}
	if !strings.Contains(body, "  └── Electric") {
		t.Errorf("expected admin category list to contain indented '  └── Electric', body:\n%s", body)
	}

	// Step 6: Verify storefront /category/vehicles returns the product assigned to Electric (cascading descendant search)
	req = httptest.NewRequest("GET", "/category/vehicles", nil)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on GET /category/vehicles, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `category=vehicles`) {
		t.Errorf("expected /category/vehicles page to reference vehicles category grid, body:\n%s", rec.Body.String())
	}

	// Verify the API returns the cascading product CyberTruck for vehicles
	req = httptest.NewRequest("GET", "/api/v1/products?category=vehicles", nil)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on GET /api/v1/products?category=vehicles, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "CyberTruck") {
		t.Errorf("expected cascading search for /api/v1/products?category=vehicles to return 'CyberTruck', body:\n%s", rec.Body.String())
	}

	// Step 7: Verify storefront /category/vehicles/cars/electric returns product & breadcrumbs
	req = httptest.NewRequest("GET", "/category/vehicles/cars/electric", nil)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on GET /category/vehicles/cars/electric, got %d", rec.Code)
	}
	body = rec.Body.String()
	if !strings.Contains(body, "/category/vehicles") || !strings.Contains(body, "/category/vehicles/cars") || !strings.Contains(body, "Electric") {
		t.Errorf("expected breadcrumbs in /category/vehicles/cars/electric page, body:\n%s", body)
	}

	// Verify the API returns CyberTruck for electric
	req = httptest.NewRequest("GET", "/api/v1/products?category=electric", nil)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK on GET /api/v1/products?category=electric, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "CyberTruck") {
		t.Errorf("expected product grid for /api/v1/products?category=electric to return 'CyberTruck', body:\n%s", rec.Body.String())
	}

	// Step 8: Verify attempting to delete Vehicles is rejected with flash error
	req = httptest.NewRequest("POST", "/admin/categories/vehicles/delete", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected HTTP 303 SeeOther on deleting category with subcategories, got %d", rec.Code)
	}

	// Follow redirect to check flash message on /admin/categories
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.AddCookie(adminCookie)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	body = rec.Body.String()
	if !strings.Contains(body, "Cannot delete category because it has subcategories") {
		t.Errorf("expected flash error 'Cannot delete category because it has subcategories', got:\n%s", body)
	}

	// Step 9: Delete Electric, then Cars, then Vehicles successfully
	deleteOrder := []string{"electric", "cars", "vehicles"}
	for _, catID := range deleteOrder {
		req = httptest.NewRequest("POST", "/admin/categories/"+catID+"/delete", nil)
		req.AddCookie(adminCookie)
		rec = httptest.NewRecorder()
		app.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected HTTP 303 SeeOther on deleting category %s, got %d", catID, rec.Code)
		}
	}

	// Verify categories are removed from admin category list
	req = httptest.NewRequest("GET", "/admin/categories", nil)
	req.AddCookie(adminCookie)
	rec = httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	body = rec.Body.String()
	for _, catName := range []string{"Vehicles", "Cars", "Electric"} {
		if strings.Contains(body, catName) {
			t.Errorf("expected admin category list to no longer contain '%s', body:\n%s", catName, body)
		}
	}
}
