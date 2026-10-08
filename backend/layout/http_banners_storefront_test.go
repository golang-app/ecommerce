package layout_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	banneradapter "github.com/bkielbasa/go-ecommerce/backend/banner/adapter"
	bannerapp "github.com/bkielbasa/go-ecommerce/backend/banner/app"
	bannerdomain "github.com/bkielbasa/go-ecommerce/backend/banner/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/layout"
	pcadapter "github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type bannersStorefrontTestApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
	bannerSrv  *bannerapp.Service
}

func newBannersStorefrontTestApp(t *testing.T) bannersStorefrontTestApp {
	t.Helper()

	if origWd, err := os.Getwd(); err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)

	bannerRepo := banneradapter.NewInMemory()
	bannerSrv := bannerapp.NewService(bannerRepo)

	pcStorage := pcadapter.NewInMemory()
	catalogSrv := pcapp.NewProductService(pcStorage)

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, nil, nil, nil, nil, nil, nil, dummyReviewsService{}, nil, nil, nil, nil, bannerSrv, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		false,
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return bannersStorefrontTestApp{
		router:     router,
		catalogSrv: &catalogSrv,
		bannerSrv:  bannerSrv,
	}
}

func TestStorefront_ActiveAnnouncementBanner(t *testing.T) {
	is := is.New(t)
	app := newBannersStorefrontTestApp(t)

	b, err := bannerdomain.NewBanner("b-ann-1", bannerdomain.SlotTopAnnouncement, "Flash Sale 20% Off", 1)
	is.NoErr(err)
	b.Body = "Use promo code at checkout"
	b.PromoCode = "FLASH20"
	b.LinkURL = "/category/sale"
	b.LinkText = "Shop Sale"
	err = app.bannerSrv.Create(context.Background(), b)
	is.NoErr(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "Flash Sale 20% Off"))
	is.True(strings.Contains(body, "FLASH20"))
	is.True(strings.Contains(body, "/category/sale"))
}

func TestStorefront_InactiveAnnouncementBanner(t *testing.T) {
	is := is.New(t)
	app := newBannersStorefrontTestApp(t)

	b, err := bannerdomain.NewBanner("b-ann-2", bannerdomain.SlotTopAnnouncement, "Expired Sale", 1)
	is.NoErr(err)
	b.IsActive = false
	err = app.bannerSrv.Create(context.Background(), b)
	is.NoErr(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(!strings.Contains(body, "Expired Sale"))
	is.True(strings.Contains(body, "Free shipping on orders over $50"))
}

func TestStorefront_ActiveHeroBanner(t *testing.T) {
	is := is.New(t)
	app := newBannersStorefrontTestApp(t)

	b, err := bannerdomain.NewBanner("b-hero-1", bannerdomain.SlotHomepageHero, "Summer Collection 2026", 1)
	is.NoErr(err)
	b.Body = "Explore our latest arrivals for summer"
	b.ImageURL = "/static/images/hero-summer.jpg"
	b.LinkURL = "/category/summer"
	b.LinkText = "Explore Collection"
	err = app.bannerSrv.Create(context.Background(), b)
	is.NoErr(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "Summer Collection 2026"))
	is.True(strings.Contains(body, "Explore our latest arrivals for summer"))
	is.True(strings.Contains(body, "/static/images/hero-summer.jpg"))
}

func TestStorefront_ActiveTrustBadges(t *testing.T) {
	is := is.New(t)
	app := newBannersStorefrontTestApp(t)

	b1, err := bannerdomain.NewBanner("b-trust-1", bannerdomain.SlotProductGuarantee, "Free Worldwide Shipping", 1)
	is.NoErr(err)
	b1.Body = "On all orders above $100"
	b1.Icon = "truck"
	err = app.bannerSrv.Create(context.Background(), b1)
	is.NoErr(err)

	b2, err := bannerdomain.NewBanner("b-trust-2", bannerdomain.SlotProductGuarantee, "30-Day Money Back", 2)
	is.NoErr(err)
	b2.Body = "No questions asked return policy"
	err = app.bannerSrv.Create(context.Background(), b2)
	is.NoErr(err)

	err = app.catalogSrv.Add(context.Background(), "prod-1", "Test Item", "Desc", 1000, "USD", "")
	is.NoErr(err)

	// Test homepage
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	bodyHome := rec.Body.String()
	is.True(strings.Contains(bodyHome, "Free Worldwide Shipping"))
	is.True(strings.Contains(bodyHome, "30-Day Money Back"))

	// Test product page
	reqProd := httptest.NewRequest(http.MethodGet, "/product/prod-1", nil)
	recProd := httptest.NewRecorder()
	app.router.ServeHTTP(recProd, reqProd)

	is.Equal(recProd.Code, http.StatusOK)
	bodyProd := recProd.Body.String()
	is.True(strings.Contains(bodyProd, "Free Worldwide Shipping"))
	is.True(strings.Contains(bodyProd, "30-Day Money Back"))
}

func TestStorefront_EmptyBannersFallback(t *testing.T) {
	is := is.New(t)
	app := newBannersStorefrontTestApp(t)

	err := app.catalogSrv.Add(context.Background(), "prod-1", "Test Item", "Desc", 1000, "USD", "")
	is.NoErr(err)

	// Homepage check
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	bodyHome := rec.Body.String()
	is.True(strings.Contains(bodyHome, "Free shipping on orders over $50"))
	is.True(strings.Contains(bodyHome, "new arrivals"))
	is.True(strings.Contains(bodyHome, "Estimated Delivery"))
	is.True(strings.Contains(bodyHome, "Hassle-Free Returns"))
	is.True(strings.Contains(bodyHome, "2-Year Warranty"))

	// Product page check
	reqProd := httptest.NewRequest(http.MethodGet, "/product/prod-1", nil)
	recProd := httptest.NewRecorder()
	app.router.ServeHTTP(recProd, reqProd)

	is.Equal(recProd.Code, http.StatusOK)
	bodyProd := recProd.Body.String()
	is.True(strings.Contains(bodyProd, "Free shipping on orders over $50"))
	is.True(strings.Contains(bodyProd, "Estimated Delivery"))
}
