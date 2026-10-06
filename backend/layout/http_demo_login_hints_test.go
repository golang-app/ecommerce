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

type demoLoginAdminAuthService struct{}

func (f demoLoginAdminAuthService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoLoginAdminAuthService) Logout(ctx context.Context, token string) error { return nil }
func (f demoLoginAdminAuthService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f demoLoginAdminAuthService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f demoLoginAdminAuthService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return false, nil
}
func (f demoLoginAdminAuthService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

func newLoginHintTestRouter(t *testing.T, demoMode bool) *mux.Router {
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
	adminAuth := demoLoginAdminAuthService{}

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

func TestDemoLoginHints(t *testing.T) {
	is := is.New(t)

	// --- Customer Login Page ---
	rDemo := newLoginHintTestRouter(t, true)
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	rDemo.ServeHTTP(rec, req)
	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "demo-credentials-box"))
	is.True(strings.Contains(body, "customer@example.com"))
	is.True(strings.Contains(body, "Customer123!"))

	rProd := newLoginHintTestRouter(t, false)
	reqProd := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	recProd := httptest.NewRecorder()
	rProd.ServeHTTP(recProd, reqProd)
	is.Equal(recProd.Code, http.StatusOK)
	bodyProd := recProd.Body.String()
	is.True(!strings.Contains(bodyProd, "demo-credentials-box"))

	// --- Admin Login Page ---
	recAdminDemo := httptest.NewRecorder()
	reqAdminDemo := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rDemo.ServeHTTP(recAdminDemo, reqAdminDemo)
	is.Equal(recAdminDemo.Code, http.StatusOK)
	bodyAdminDemo := recAdminDemo.Body.String()
	is.True(strings.Contains(bodyAdminDemo, "admin-demo-credentials-box"))
	is.True(strings.Contains(bodyAdminDemo, "admin@example.com"))
	is.True(strings.Contains(bodyAdminDemo, "Admin123!"))

	recAdminProd := httptest.NewRecorder()
	reqAdminProd := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rProd.ServeHTTP(recAdminProd, reqAdminProd)
	is.Equal(recAdminProd.Code, http.StatusOK)
	bodyAdminProd := recAdminProd.Body.String()
	is.True(!strings.Contains(bodyAdminProd, "admin-demo-credentials-box"))
}
