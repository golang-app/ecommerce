package layout_test

import (
	"context"
	"encoding/json"
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
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	widgetadapter "github.com/bkielbasa/go-ecommerce/backend/widget/adapter"
	widgetapp "github.com/bkielbasa/go-ecommerce/backend/widget/app"
	widgetdomain "github.com/bkielbasa/go-ecommerce/backend/widget/domain"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type dummyReviewsService struct{}

func (dummyReviewsService) ListForProduct(ctx context.Context, productID string, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (dummyReviewsService) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]reviewsDomain.Aggregate, error) {
	return nil, nil
}
func (dummyReviewsService) HasReviewed(ctx context.Context, productID, customerID string) (bool, error) {
	return false, nil
}
func (dummyReviewsService) Submit(ctx context.Context, productID, customerID, body string, rating int) error {
	return nil
}
func (dummyReviewsService) Delete(ctx context.Context, id string) error  { return nil }
func (dummyReviewsService) Approve(ctx context.Context, id string) error { return nil }
func (dummyReviewsService) Reject(ctx context.Context, id string) error  { return nil }
func (dummyReviewsService) ListPending(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (dummyReviewsService) ListAll(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (dummyReviewsService) ListByCustomer(ctx context.Context, customerID string) ([]reviewsDomain.Review, error) {
	return nil, nil
}

type widgetsStorefrontTestApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
	widgetSrv  *widgetapp.Service
}

func newWidgetsStorefrontTestApp(t *testing.T) widgetsStorefrontTestApp {
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

	widgetRepo := widgetadapter.NewInMemory()
	widgetSrv := widgetapp.NewService(widgetRepo)

	bc := layout.New(
		logger,
		nil, catalogSrv, nil, nil, nil, nil, nil, nil, nil, dummyReviewsService{}, nil, nil, nil, nil, widgetSrv, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		false,
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return widgetsStorefrontTestApp{
		router:     router,
		catalogSrv: &catalogSrv,
		widgetSrv:  widgetSrv,
	}
}

func TestWidgetsStorefront_ZeroDefaults(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	// Zero hardcoded defaults
	is.True(!strings.Contains(body, "announcement-bar"))
	is.True(!strings.Contains(body, "trust-bar"))
	is.True(!strings.Contains(body, "home-hero"))
}

func TestWidgetsStorefront_SingleSlideHero(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	w := widgetdomain.Widget{
		ID:           "w-hero-single",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeBannerCarousel,
		Title:        "Main Hero",
		DisplayOrder: 1,
		IsActive:     true,
	}
	err := app.widgetSrv.Create(ctx, w)
	is.NoErr(err)

	s := widgetdomain.Slide{
		ID:        "s-1",
		WidgetID:  "w-hero-single",
		Title:     "Summer Sale 50% Off",
		Subtitle:  "Grab your favorite items now",
		LinkURL:   "/catalog",
		LinkText:  "Shop Summer",
		PromoCode: "SUMMER50",
		Badge:     "LIMITED",
	}
	err = app.widgetSrv.AddSlide(ctx, s)
	is.NoErr(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "widget-hero"))
	is.True(strings.Contains(body, "Summer Sale 50% Off"))
	is.True(strings.Contains(body, "SUMMER50"))
	is.True(strings.Contains(body, "promo-code-btn"))
	is.True(strings.Contains(body, "/catalog"))
	is.True(strings.Contains(body, "Shop Summer"))
	is.True(strings.Contains(body, "LIMITED"))
}

func TestWidgetsStorefront_MultiSlideCarousel(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	w := widgetdomain.Widget{
		ID:           "w-hero-multi",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeBannerCarousel,
		Title:        "Featured Carousel",
		DisplayOrder: 1,
		IsActive:     true,
	}
	err := app.widgetSrv.Create(ctx, w)
	is.NoErr(err)

	s1 := widgetdomain.Slide{
		ID:           "s-1",
		WidgetID:     "w-hero-multi",
		Title:        "First Slide Title",
		DisplayOrder: 1,
	}
	s2 := widgetdomain.Slide{
		ID:           "s-2",
		WidgetID:     "w-hero-multi",
		Title:        "Second Slide Title",
		DisplayOrder: 2,
	}
	is.NoErr(app.widgetSrv.AddSlide(ctx, s1))
	is.NoErr(app.widgetSrv.AddSlide(ctx, s2))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "carousel-widget"))
	is.True(strings.Contains(body, "carousel-widget__arrow"))
	is.True(strings.Contains(body, "carousel-widget__dots"))
	is.True(strings.Contains(body, "First Slide Title"))
	is.True(strings.Contains(body, "Second Slide Title"))
}

func TestWidgetsStorefront_NewArrivalsWidget(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	err := app.catalogSrv.Add(ctx, "prod-new-1", "Cool Sneakers", "Latest arrival", 12000, "USD", "/sneakers.jpg")
	is.NoErr(err)

	cfg, _ := json.Marshal(widgetdomain.NewArrivalsConfig{ProductLimit: 4})
	w := widgetdomain.Widget{
		ID:           "w-new-arrivals",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeNewArrivals,
		Title:        "Fresh Drops",
		Config:       cfg,
		DisplayOrder: 1,
		IsActive:     true,
	}
	is.NoErr(app.widgetSrv.Create(ctx, w))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "widget-products"))
	is.True(strings.Contains(body, "Fresh Drops"))
	is.True(strings.Contains(body, "Cool Sneakers"))
}

func TestWidgetsStorefront_CategoryProductsWidget(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	err := app.catalogSrv.CreateCategory(ctx, "Audio", "audio", "")
	is.NoErr(err)

	cats, err := app.catalogSrv.Categories(ctx)
	is.NoErr(err)

	var audioCatID string
	for _, c := range cats {
		if c.Slug() == "audio" {
			audioCatID = c.ID()
			break
		}
	}
	is.True(audioCatID != "")

	err = app.catalogSrv.Add(ctx, "prod-headphone", "Wireless Headphones", "Noise cancelling", 25000, "USD", "/headphones.jpg")
	is.NoErr(err)
	err = app.catalogSrv.SetProductCategories(ctx, "prod-headphone", []string{audioCatID})
	is.NoErr(err)

	cfg, _ := json.Marshal(widgetdomain.CategoryProductsConfig{CategoryID: audioCatID, ProductLimit: 4})
	w := widgetdomain.Widget{
		ID:           "w-cat-audio",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeCategoryProducts,
		Title:        "Best Audio Gear",
		Config:       cfg,
		DisplayOrder: 1,
		IsActive:     true,
	}
	is.NoErr(app.widgetSrv.Create(ctx, w))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(strings.Contains(body, "widget-products"))
	is.True(strings.Contains(body, "Best Audio Gear"))
	is.True(strings.Contains(body, "Wireless Headphones"))
}

func TestWidgetsStorefront_DisplayOrderSorting(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	w2 := widgetdomain.Widget{
		ID:           "w-second",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeBannerCarousel,
		Title:        "SECOND_WIDGET_TITLE",
		DisplayOrder: 20,
		IsActive:     true,
	}
	s2 := widgetdomain.Slide{ID: "s2", WidgetID: "w-second", Title: "SECOND_WIDGET_TITLE"}
	is.NoErr(app.widgetSrv.Create(ctx, w2))
	is.NoErr(app.widgetSrv.AddSlide(ctx, s2))

	w1 := widgetdomain.Widget{
		ID:           "w-first",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeBannerCarousel,
		Title:        "FIRST_WIDGET_TITLE",
		DisplayOrder: 10,
		IsActive:     true,
	}
	s1 := widgetdomain.Slide{ID: "s1", WidgetID: "w-first", Title: "FIRST_WIDGET_TITLE"}
	is.NoErr(app.widgetSrv.Create(ctx, w1))
	is.NoErr(app.widgetSrv.AddSlide(ctx, s1))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	pos1 := strings.Index(body, "FIRST_WIDGET_TITLE")
	pos2 := strings.Index(body, "SECOND_WIDGET_TITLE")

	is.True(pos1 != -1)
	is.True(pos2 != -1)
	is.True(pos1 < pos2)
}

func TestWidgetsStorefront_InactiveWidgetsFiltered(t *testing.T) {
	is := is.New(t)
	app := newWidgetsStorefrontTestApp(t)
	ctx := context.Background()

	w := widgetdomain.Widget{
		ID:           "w-inactive",
		Page:         widgetdomain.PageHome,
		Zone:         widgetdomain.ZoneBody,
		Type:         widgetdomain.TypeBannerCarousel,
		Title:        "INACTIVE_WIDGET_TITLE",
		DisplayOrder: 1,
		IsActive:     false,
	}
	s := widgetdomain.Slide{ID: "s1", WidgetID: "w-inactive", Title: "INACTIVE_SLIDE_TITLE"}
	is.NoErr(app.widgetSrv.Create(ctx, w))
	is.NoErr(app.widgetSrv.AddSlide(ctx, s))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	is.True(!strings.Contains(body, "INACTIVE_WIDGET_TITLE"))
	is.True(!strings.Contains(body, "INACTIVE_SLIDE_TITLE"))
}
