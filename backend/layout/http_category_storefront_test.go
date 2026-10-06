package layout_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type testApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
}

func (a *testApp) Router() *mux.Router {
	return a.router
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	if origWd, err := os.Getwd(); err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage)

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return &testApp{
		router:     router,
		catalogSrv: &catalogSrv,
	}
}

func seedHierarchy(t *testing.T, app *testApp) {
	t.Helper()
	ctx := context.Background()
	err := app.catalogSrv.CreateCategory(ctx, "Clothing", "clothing", "")
	if err != nil {
		t.Fatalf("failed to create clothing: %v", err)
	}

	cats, err := app.catalogSrv.Categories(ctx)
	if err != nil {
		t.Fatalf("failed to list categories: %v", err)
	}
	var clothingID string
	for _, c := range cats {
		if c.Slug() == "clothing" {
			clothingID = c.ID()
		}
	}

	err = app.catalogSrv.CreateCategory(ctx, "Men", "men", clothingID)
	if err != nil {
		t.Fatalf("failed to create men: %v", err)
	}

	cats, err = app.catalogSrv.Categories(ctx)
	if err != nil {
		t.Fatalf("failed to list categories: %v", err)
	}
	var menID string
	for _, c := range cats {
		if c.Slug() == "men" {
			menID = c.ID()
		}
	}

	err = app.catalogSrv.CreateCategory(ctx, "Shirts", "shirts", menID)
	if err != nil {
		t.Fatalf("failed to create shirts: %v", err)
	}
}

func TestStorefront_HierarchicalCategoryPath(t *testing.T) {
	app := newTestApp(t)
	seedHierarchy(t, app)

	req := httptest.NewRequest("GET", "/category/clothing/men/shirts", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	// Check breadcrumb links exist
	if !strings.Contains(body, `/category/clothing`) {
		t.Errorf("expected breadcrumb link to /category/clothing")
	}
	if !strings.Contains(body, `/category/clothing/men`) {
		t.Errorf("expected breadcrumb link to /category/clothing/men")
	}
	if !strings.Contains(body, `shirts`) {
		t.Errorf("expected active category shirts in body")
	}
}

func TestStorefront_SubcategoryPills(t *testing.T) {
	app := newTestApp(t)
	seedHierarchy(t, app)

	req := httptest.NewRequest("GET", "/category/clothing", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Men should appear as direct subcategory pill/link of Clothing
	if !strings.Contains(body, `/category/clothing/men`) {
		t.Errorf("expected subcategory link to /category/clothing/men")
	}
}
