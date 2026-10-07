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
	reviewsDomain "github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
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
func (dummyReviewsService) Delete(ctx context.Context, id string) error { return nil }
func (dummyReviewsService) Approve(ctx context.Context, id string) error { return nil }
func (dummyReviewsService) Reject(ctx context.Context, id string) error { return nil }
func (dummyReviewsService) ListPending(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (dummyReviewsService) ListAll(ctx context.Context, limit int) ([]reviewsDomain.Review, error) {
	return nil, nil
}
func (dummyReviewsService) ListByCustomer(ctx context.Context, customerID string) ([]reviewsDomain.Review, error) {
	return nil, nil
}

type announcementTrustTestApp struct {
	router     *mux.Router
	catalogSrv *pcapp.ProductService
}

func newAnnouncementTrustTestApp(t *testing.T) announcementTrustTestApp {
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
		nil, catalogSrv, nil, nil, nil, nil, nil, nil, nil, dummyReviewsService{}, nil, nil, nil, nil, nil,
		"", []byte("secret-key-32-bytes-long-12345"), false, false, nil, "", fx.Rates{}, "", nil, nil, "",
		false,
	)

	router := mux.NewRouter()
	if mr, ok := bc.(interface{ MuxRegister(r *mux.Router) }); ok {
		mr.MuxRegister(router)
	} else {
		t.Fatalf("boundedContext does not implement MuxRegister")
	}

	return announcementTrustTestApp{
		router:     router,
		catalogSrv: &catalogSrv,
	}
}

func TestAnnouncementBar_Storefront(t *testing.T) {
	is := is.New(t)
	app := newAnnouncementTrustTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	// Semantic markup and accessible attributes
	is.True(strings.Contains(body, `<aside class="announcement-bar"`))
	is.True(strings.Contains(body, `role="region"`))
	is.True(strings.Contains(body, `aria-label="Announcement"`))

	// Free shipping highlight
	is.True(strings.Contains(body, "Free shipping on orders over $50"))

	// Promo code with 1-click copy
	is.True(strings.Contains(body, "WELCOME10"))
	is.True(strings.Contains(body, "Use code"))
	is.True(strings.Contains(body, "for 10% off"))
	is.True(strings.Contains(body, "Copy Code"))
	is.True(strings.Contains(body, "announcement-bar__code-btn"))

	// Dismiss button and anti-flicker script
	is.True(strings.Contains(body, "announcement-bar__close"))
	is.True(strings.Contains(body, "dismissAnnouncementBar()"))
	is.True(strings.Contains(body, "sessionStorage.getItem('dismissed_announcement_v1')"))
	is.True(strings.Contains(body, "sessionStorage.setItem('dismissed_announcement_v1'"))

	// Clipboard copy helper
	is.True(strings.Contains(body, "navigator.clipboard"))
	is.True(strings.Contains(body, "✓ Copied!"))
}

func TestTrustBadges_Homepage(t *testing.T) {
	is := is.New(t)
	app := newAnnouncementTrustTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	// Value block container
	is.True(strings.Contains(body, `class="trust-bar"`))

	// Estimated Delivery
	is.True(strings.Contains(body, "Estimated Delivery"))
	is.True(strings.Contains(body, "Arrives in 2–4 business days • Orders before 2 PM ship today"))

	// Hassle-Free Returns
	is.True(strings.Contains(body, "Hassle-Free Returns"))
	is.True(strings.Contains(body, "Prepaid return label included • 30-day money-back guarantee"))

	// 2-Year Warranty
	is.True(strings.Contains(body, "2-Year Warranty"))
	is.True(strings.Contains(body, "Full replacement warranty included at no extra cost"))
}

func TestTrustBadges_ProductPage(t *testing.T) {
	is := is.New(t)
	app := newAnnouncementTrustTestApp(t)
	ctx := context.Background()

	err := app.catalogSrv.Add(ctx, "prod-trust-test", "Modern Desk", "Sleek minimalist wooden desk", 19900, "USD", "")
	is.NoErr(err)

	req := httptest.NewRequest(http.MethodGet, "/product/prod-trust-test", nil)
	rec := httptest.NewRecorder()
	app.router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	// Base layout announcement bar is present
	is.True(strings.Contains(body, `class="announcement-bar"`))
	is.True(strings.Contains(body, "WELCOME10"))

	// Product page contains trust badges
	is.True(strings.Contains(body, `class="trust-bar"`))
	is.True(strings.Contains(body, "Estimated Delivery"))
	is.True(strings.Contains(body, "Arrives in 2–4 business days • Orders before 2 PM ship today"))
	is.True(strings.Contains(body, "Hassle-Free Returns"))
	is.True(strings.Contains(body, "Prepaid return label included • 30-day money-back guarantee"))
	is.True(strings.Contains(body, "2-Year Warranty"))
	is.True(strings.Contains(body, "Full replacement warranty included at no extra cost"))
}
