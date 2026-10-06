package layout_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

type demoModeAdminAuthService struct{}

func (f demoModeAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoModeAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f demoModeAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoModeAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f demoModeAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f demoModeAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

func newDemoModeTestApp(t *testing.T, demoMode bool) *mux.Router {
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
	adminAuth := demoModeAdminAuthService{}

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

func TestDemoMode_Wiring(t *testing.T) {
	is := is.New(t)

	routerTrue := newDemoModeTestApp(t, true)
	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	routerTrue.ServeHTTP(rec, req)
	is.Equal(rec.Code, http.StatusOK)

	routerFalse := newDemoModeTestApp(t, false)
	reqFalse := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	recFalse := httptest.NewRecorder()
	routerFalse.ServeHTTP(recFalse, reqFalse)
	is.Equal(recFalse.Code, http.StatusOK)
}
