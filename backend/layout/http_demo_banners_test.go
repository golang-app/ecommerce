package layout_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type demoBannerAdminAuthService struct{}

func (f demoBannerAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoBannerAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f demoBannerAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoBannerAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f demoBannerAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f demoBannerAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

func newBannerTestRouter(t *testing.T, demoMode bool) *mux.Router {
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
	adminAuth := demoBannerAdminAuthService{}

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, adminAuth, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		demoMode,
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return router
}

func TestDemoBanners_Storefront(t *testing.T) {
	is := is.New(t)

	// In demo mode: storefront contains demo-banner
	rDemo := newBannerTestRouter(t, true)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	rDemo.ServeHTTP(rec, req)
	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "demo-banner"))
	is.True(strings.Contains(body, "DEMO MODE"))
	is.True(strings.Contains(body, "This is an interactive demo store"))

	// In non-demo mode: storefront does NOT contain demo-banner
	rProd := newBannerTestRouter(t, false)
	reqProd := httptest.NewRequest(http.MethodGet, "/", nil)
	recProd := httptest.NewRecorder()
	rProd.ServeHTTP(recProd, reqProd)
	is.Equal(recProd.Code, http.StatusOK)
	bodyProd := recProd.Body.String()
	is.True(!strings.Contains(bodyProd, "demo-banner"))
}

func TestDemoBanners_Admin(t *testing.T) {
	is := is.New(t)

	// Admin auth view in demo mode contains admin-demo-banner
	rDemo := newBannerTestRouter(t, true)
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	rDemo.ServeHTTP(rec, req)
	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "admin-demo-banner"))
	is.True(strings.Contains(body, "DEMO MODE"))

	// Admin auth view in non-demo mode does NOT contain admin-demo-banner
	rProd := newBannerTestRouter(t, false)
	reqProd := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	recProd := httptest.NewRecorder()
	rProd.ServeHTTP(recProd, reqProd)
	is.Equal(recProd.Code, http.StatusOK)
	bodyProd := recProd.Body.String()
	is.True(!strings.Contains(bodyProd, "admin-demo-banner"))
}
