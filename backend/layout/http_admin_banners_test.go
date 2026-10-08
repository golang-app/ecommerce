package layout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	banneradapter "github.com/bkielbasa/go-ecommerce/backend/banner/adapter"
	bannerapp "github.com/bkielbasa/go-ecommerce/backend/banner/app"
	bannerdomain "github.com/bkielbasa/go-ecommerce/backend/banner/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

func newBannerTestRouter(srv bannerService) *mux.Router {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	h := httpHandler{
		bannerSrv:    srv,
		adminAuthSrv: adminAuth,
		rates:        rates,
	}

	bc := boundedContext{
		handler: h,
		logger:  logger,
	}
	r := mux.NewRouter()
	bc.MuxRegister(r)
	return r
}

func TestAdminBanners_Unauthenticated(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)
	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/admin/banners", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/login")
}

func TestAdminBanners_List(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)

	b1, err := bannerdomain.NewBanner("banner-1", bannerdomain.SlotTopAnnouncement, "Free Shipping Over $50", 1)
	is.NoErr(err)
	b1.PromoCode = "FREESHIP"
	_ = srv.Create(context.Background(), b1)

	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/admin/banners", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "banners"))
	is.True(strings.Contains(body, "Free Shipping Over $50"))
	is.True(strings.Contains(body, "FREESHIP"))
	is.True(strings.Contains(body, "+ new banner"))
}

func TestAdminBanners_NewForm(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)
	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/admin/banners/new", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "new banner"))
	is.True(strings.Contains(body, "create banner"))
}

func TestAdminBanners_Create(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)
	router := newBannerTestRouter(srv)

	form := url.Values{}
	form.Set("slot", "top_announcement")
	form.Set("title", "Spring Discount 15%")
	form.Set("body", "Special spring offers for members")
	form.Set("link_url", "/products")
	form.Set("link_text", "Shop Spring")
	form.Set("promo_code", "SPRING15")
	form.Set("icon", "tag")
	form.Set("display_order", "10")
	form.Set("is_active", "on")

	req := httptest.NewRequest(http.MethodPost, "/admin/banners", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/banners")

	all, err := srv.ListAll(context.Background())
	is.NoErr(err)
	is.Equal(len(all), 1)
	is.Equal(all[0].Title, "Spring Discount 15%")
	is.Equal(all[0].Slot, bannerdomain.SlotTopAnnouncement)
	is.Equal(all[0].PromoCode, "SPRING15")
	is.Equal(all[0].DisplayOrder, 10)
	is.Equal(all[0].IsActive, true)
}

func TestAdminBanners_EditForm(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)

	b, err := bannerdomain.NewBanner("b-edit-1", bannerdomain.SlotHomepageHero, "Hero Headline", 5)
	is.NoErr(err)
	b.Body = "Hero subtitle description"
	_ = srv.Create(context.Background(), b)

	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/admin/banners/b-edit-1/edit", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "edit banner"))
	is.True(strings.Contains(body, "Hero Headline"))
	is.True(strings.Contains(body, "Hero subtitle description"))
}

func TestAdminBanners_Update(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)

	b, err := bannerdomain.NewBanner("b-up-1", bannerdomain.SlotProductGuarantee, "Old Guarantee", 2)
	is.NoErr(err)
	_ = srv.Create(context.Background(), b)

	router := newBannerTestRouter(srv)

	form := url.Values{}
	form.Set("slot", "product_guarantee")
	form.Set("title", "2-Year Full Warranty")
	form.Set("body", "Replacement included")
	form.Set("display_order", "1")
	form.Set("icon", "shield")
	form.Set("is_active", "on")

	req := httptest.NewRequest(http.MethodPost, "/admin/banners/b-up-1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/banners")

	updated, err := srv.Get(context.Background(), "b-up-1")
	is.NoErr(err)
	is.Equal(updated.Title, "2-Year Full Warranty")
	is.Equal(updated.Body, "Replacement included")
	is.Equal(updated.Icon, "shield")
	is.Equal(updated.DisplayOrder, 1)
}

func TestAdminBanners_Toggle(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)

	b, err := bannerdomain.NewBanner("b-tog-1", bannerdomain.SlotTopAnnouncement, "Announcement Bar", 1)
	is.NoErr(err)
	is.Equal(b.IsActive, true)
	_ = srv.Create(context.Background(), b)

	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodPost, "/admin/banners/b-tog-1/toggle", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/banners")

	updated, err := srv.Get(context.Background(), "b-tog-1")
	is.NoErr(err)
	is.Equal(updated.IsActive, false)
}

func TestAdminBanners_Delete(t *testing.T) {
	setupTestEnvironment(t)
	is := is.New(t)

	repo := banneradapter.NewInMemory()
	srv := bannerapp.NewService(repo)

	b, err := bannerdomain.NewBanner("b-del-1", bannerdomain.SlotHomepageHero, "To Delete", 1)
	is.NoErr(err)
	_ = srv.Create(context.Background(), b)

	router := newBannerTestRouter(srv)

	req := httptest.NewRequest(http.MethodPost, "/admin/banners/b-del-1/delete", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/banners")

	_, err = srv.Get(context.Background(), "b-del-1")
	is.True(err != nil)
}
