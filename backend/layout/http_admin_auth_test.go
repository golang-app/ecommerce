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

type adminAuthTestService struct {
	mustChange bool
}

func (f adminAuthTestService) Login(ctx context.Context, email, password string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f adminAuthTestService) Logout(ctx context.Context, token string) error { return nil }
func (f adminAuthTestService) FindByToken(ctx context.Context, token string) (*authDomain.Session, error) {
	return authDomain.NewSession("admin-token", "admin@example.com", time.Now().Add(time.Hour)), nil
}
func (f adminAuthTestService) ChangePassword(ctx context.Context, email, oldPassword, newPassword string) error {
	return nil
}
func (f adminAuthTestService) MustChangePassword(ctx context.Context, email string) (bool, error) {
	return f.mustChange, nil
}
func (f adminAuthTestService) FindByID(ctx context.Context, id string) (authAdapter.Admin, error) {
	return authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}, nil
}

type adminAuthTestApp struct {
	router *mux.Router
}

func newAdminAuthTestApp(t *testing.T, auth adminAuthTestService) *adminAuthTestApp {
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

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, auth, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return &adminAuthTestApp{router: router}
}

func TestAdminLoginPage_AdminStyles(t *testing.T) {
	app := newAdminAuthTestApp(t, adminAuthTestService{})

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on /admin/login, got %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Must use admin.css, NOT storefront main.css
	if !strings.Contains(body, `<link rel="stylesheet" href="/static/admin.css">`) {
		t.Errorf("expected /admin/login to include /static/admin.css")
	}
	if strings.Contains(body, `<link rel="stylesheet" href="/static/main.css">`) {
		t.Errorf("expected /admin/login to NOT include storefront /static/main.css")
	}

	// 2. Must contain admin branding & auth card
	if !strings.Contains(body, "admin-auth-card") {
		t.Errorf("expected /admin/login to contain admin-auth-card container")
	}
	if !strings.Contains(body, "Store Admin") {
		t.Errorf("expected /admin/login to contain 'Store Admin' brand")
	}

	// 3. Must contain login form with CSRF and inputs
	if !strings.Contains(body, `action="/admin/login"`) {
		t.Errorf("expected /admin/login form with action='/admin/login'")
	}
	if !strings.Contains(body, `name="csrf_token"`) {
		t.Errorf("expected /admin/login form to include csrf_token")
	}
	if !strings.Contains(body, `name="email"`) {
		t.Errorf("expected /admin/login form to include email field")
	}
	if !strings.Contains(body, `name="password"`) {
		t.Errorf("expected /admin/login form to include password field")
	}

	// 4. Must provide link back to store
	if !strings.Contains(body, `href="/"`) {
		t.Errorf("expected /admin/login to contain a link back to store")
	}

	// 5. Must NOT leak storefront UI (e.g. cart or storefront header)
	if strings.Contains(body, `<header class="header">`) {
		t.Errorf("expected /admin/login to NOT contain storefront header")
	}
	if strings.Contains(body, `cart-link`) {
		t.Errorf("expected /admin/login to NOT contain cart link")
	}
}

func TestAdminChangePasswordPage_AdminStyles(t *testing.T) {
	app := newAdminAuthTestApp(t, adminAuthTestService{mustChange: true})

	// Obtain admin session first
	form := url.Values{"email": {"admin@example.com"}, "password": {"secret"}}
	loginReq := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	app.router.ServeHTTP(loginRec, loginReq)

	var adminCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "ecommerce_admin" {
			adminCookie = c
			break
		}
	}
	if adminCookie == nil {
		t.Fatalf("failed to obtain admin session cookie")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/change-password", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 on /admin/change-password, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Must use admin.css, NOT storefront main.css
	if !strings.Contains(body, `<link rel="stylesheet" href="/static/admin.css">`) {
		t.Errorf("expected /admin/change-password to include /static/admin.css")
	}
	if strings.Contains(body, `<link rel="stylesheet" href="/static/main.css">`) {
		t.Errorf("expected /admin/change-password to NOT include storefront /static/main.css")
	}
	if !strings.Contains(body, "admin-auth-card") {
		t.Errorf("expected /admin/change-password to contain admin-auth-card container")
	}
}
