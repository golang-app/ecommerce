package layout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	authAdapter "github.com/bkielbasa/go-ecommerce/backend/auth/adapter"
	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	widgetdomain "github.com/bkielbasa/go-ecommerce/backend/widget/domain"
	"github.com/gorilla/mux"
	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

type mockWidgetService struct {
	mu      sync.RWMutex
	widgets map[string]widgetdomain.Widget
}

func newMockWidgetService() *mockWidgetService {
	return &mockWidgetService{
		widgets: make(map[string]widgetdomain.Widget),
	}
}

func (m *mockWidgetService) ListAll(ctx context.Context) ([]widgetdomain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []widgetdomain.Widget
	for _, w := range m.widgets {
		result = append(result, w)
	}
	return result, nil
}

func (m *mockWidgetService) ListActiveByPages(ctx context.Context, pages ...widgetdomain.Page) ([]widgetdomain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pageMap := make(map[widgetdomain.Page]bool)
	for _, p := range pages {
		pageMap[p] = true
	}

	var result []widgetdomain.Widget
	for _, w := range m.widgets {
		if w.IsActive && pageMap[w.Page] {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *mockWidgetService) Get(ctx context.Context, id string) (widgetdomain.Widget, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w, ok := m.widgets[id]
	if !ok {
		return widgetdomain.Widget{}, widgetdomain.ErrWidgetNotFound
	}
	return w, nil
}

func (m *mockWidgetService) Create(ctx context.Context, w widgetdomain.Widget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.widgets[w.ID]; ok {
		return errors.New("widget already exists")
	}
	m.widgets[w.ID] = w
	return nil
}

func (m *mockWidgetService) Update(ctx context.Context, w widgetdomain.Widget) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.widgets[w.ID]; !ok {
		return widgetdomain.ErrWidgetNotFound
	}
	m.widgets[w.ID] = w
	return nil
}

func (m *mockWidgetService) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.widgets[id]; !ok {
		return widgetdomain.ErrWidgetNotFound
	}
	delete(m.widgets, id)
	return nil
}

func (m *mockWidgetService) ToggleActive(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.widgets[id]
	if !ok {
		return widgetdomain.ErrWidgetNotFound
	}
	w.IsActive = !w.IsActive
	m.widgets[id] = w
	return nil
}

func (m *mockWidgetService) AddSlide(ctx context.Context, s widgetdomain.Slide) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.widgets[s.WidgetID]
	if !ok {
		return widgetdomain.ErrWidgetNotFound
	}
	w.Slides = append(w.Slides, s)
	m.widgets[s.WidgetID] = w
	return nil
}

func (m *mockWidgetService) DeleteSlide(ctx context.Context, widgetID, slideID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.widgets[widgetID]
	if !ok {
		return widgetdomain.ErrWidgetNotFound
	}

	found := false
	var newSlides []widgetdomain.Slide
	for _, slide := range w.Slides {
		if slide.ID == slideID {
			found = true
			continue
		}
		newSlides = append(newSlides, slide)
	}

	if !found {
		return widgetdomain.ErrSlideNotFound
	}

	w.Slides = newSlides
	m.widgets[widgetID] = w
	return nil
}

type mockCatalogServiceForWidgets struct {
	categories []pcdomain.Category
}

func (m mockCatalogServiceForWidgets) AllProducts(ctx context.Context) ([]pcdomain.Product, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) Newest(ctx context.Context, limit int) ([]pcdomain.Product, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) Find(ctx context.Context, id string) (pcdomain.Product, error) {
	return pcdomain.Product{}, nil
}
func (m mockCatalogServiceForWidgets) FindVariant(ctx context.Context, variantID string) (pcdomain.Product, pcdomain.Variant, error) {
	return pcdomain.Product{}, pcdomain.Variant{}, nil
}
func (m mockCatalogServiceForWidgets) List(ctx context.Context, q pcapp.ProductQuery) ([]pcdomain.Product, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) Categories(ctx context.Context) ([]pcdomain.Category, error) {
	return m.categories, nil
}
func (m mockCatalogServiceForWidgets) Facets(ctx context.Context, categorySlug string) ([]pcapp.Facet, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) Add(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) AddVariantProduct(ctx context.Context, id, name, desc, currency, thumbnail string, optionTypes []pcapp.OptionTypeInput, variants []pcapp.VariantInput) error {
	return nil
}
func (m mockCatalogServiceForWidgets) AddVariantToProduct(ctx context.Context, productID, sku, image string, priceMinor int64, currency string, stock int, options map[string]string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) AddOptionType(ctx context.Context, productID, name string, values []string, variantDefault string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateOptionType(ctx context.Context, productID, currentName, newName string, values []string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteOptionType(ctx context.Context, productID, name string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateVariant(ctx context.Context, variantID, sku, image string, priceMinor int64, currency string, stock int) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteVariant(ctx context.Context, productID, variantID string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateProduct(ctx context.Context, id, name, desc string, priceMinorUnits int64, currency, thumbnail string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteProduct(ctx context.Context, id string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) SetVariantStock(ctx context.Context, variantID string, stock int) error {
	return nil
}
func (m mockCatalogServiceForWidgets) SetProductCategories(ctx context.Context, productID string, categoryIDs []string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) SetProductAttributes(ctx context.Context, productID string, values []pcapp.AttributeAssignment) error {
	return nil
}
func (m mockCatalogServiceForWidgets) SetProductAttributeSet(ctx context.Context, productID, setID string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) ProductAttributeTypes(ctx context.Context, productID string) ([]pcdomain.AttributeType, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) AddProductImages(ctx context.Context, productID string, urls []string) ([]pcdomain.ProductImage, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) SetPrimaryProductImage(ctx context.Context, productID, imageID string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteProductImage(ctx context.Context, productID, imageID string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) ProductImages(ctx context.Context, productID string) ([]pcdomain.ProductImage, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) CreateCategory(ctx context.Context, name, slug, parentID string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateCategory(ctx context.Context, id, name, slug, parentID string, position int) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteCategory(ctx context.Context, id string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) CategoryByPath(ctx context.Context, path string) (pcdomain.Category, []pcdomain.Category, error) {
	return pcdomain.Category{}, nil, nil
}
func (m mockCatalogServiceForWidgets) AttributeTypes(ctx context.Context) ([]pcdomain.AttributeType, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) AllAttributeTypes(ctx context.Context) ([]pcdomain.AttributeType, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) CreateAttributeType(ctx context.Context, name, unit string, kind pcdomain.AttributeKind, filterable bool) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateAttributeType(ctx context.Context, id, name, unit string, kind pcdomain.AttributeKind, filterable bool, position int) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteAttributeType(ctx context.Context, id string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) AttributeSets(ctx context.Context) ([]pcdomain.AttributeSet, error) {
	return nil, nil
}
func (m mockCatalogServiceForWidgets) FindAttributeSet(ctx context.Context, id string) (pcdomain.AttributeSet, error) {
	return pcdomain.AttributeSet{}, nil
}
func (m mockCatalogServiceForWidgets) CreateAttributeSet(ctx context.Context, name string, attributeTypeIDs []string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) UpdateAttributeSet(ctx context.Context, id, name string, attributeTypeIDs []string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) DeleteAttributeSet(ctx context.Context, id string) error {
	return nil
}
func (m mockCatalogServiceForWidgets) ListStockMovements(ctx context.Context, variantID string, limit int) ([]pcdomain.StockMovement, error) {
	return nil, nil
}

func newTestWidgetApp(t *testing.T, wSrv widgetService) (*mux.Router, httpHandler) {
	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	rates := fx.New("USD", "USD", "USD:1.0", logger)
	admin := authAdapter.Admin{ID: "admin@example.com", Email: "admin@example.com", Role: "admin"}
	sess := authDomain.NewSession("admin-session-token", "admin@example.com", time.Now().Add(time.Hour))
	adminAuth := fakeAdminAuthService{admin: admin, sess: sess}

	cat1, _ := pcdomain.NewCategory("cat-shoes", "Shoes", "shoes", 1, "")
	cat2, _ := pcdomain.NewCategory("cat-shirts", "Shirts", "shirts", 2, "")
	catSrv := mockCatalogServiceForWidgets{
		categories: []pcdomain.Category{cat1, cat2},
	}

	diskStore := imagestore.NewDisk(t.TempDir(), "/uploads")

	handler := httpHandler{
		widgetSrv:    wSrv,
		catalogSrv:   catSrv,
		adminAuthSrv: adminAuth,
		imageStore:   diskStore,
		rates:        rates,
		logger:       logger,
	}

	bc := boundedContext{
		handler: handler,
		logger:  logger,
	}

	router := mux.NewRouter()
	bc.MuxRegister(router)

	return router, handler
}

func TestAdminWidgets_UnauthenticatedRedirect(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	router, _ := newTestWidgetApp(t, wSrv)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/admin/widgets"},
		{http.MethodGet, "/admin/widgets/new"},
		{http.MethodGet, "/admin/widgets/w1/edit"},
		{http.MethodPost, "/admin/widgets"},
		{http.MethodPost, "/admin/widgets/w1"},
		{http.MethodPost, "/admin/widgets/w1/toggle"},
		{http.MethodPost, "/admin/widgets/w1/delete"},
		{http.MethodPost, "/admin/widgets/w1/slides"},
		{http.MethodPost, "/admin/widgets/w1/slides/s1/delete"},
	}

	for _, rt := range routes {
		req := httptest.NewRequest(rt.method, rt.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		is.Equal(rec.Code, http.StatusSeeOther)
		is.True(strings.HasPrefix(rec.Header().Get("Location"), "/admin/login"))
	}
}

func TestAdminWidgets_List(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	w1, _ := widgetdomain.NewWidget("w1", widgetdomain.PageHome, widgetdomain.ZoneTop, widgetdomain.TypeBannerCarousel, "Home Hero Banner", 1)
	w2, _ := widgetdomain.NewWidget("w2", widgetdomain.PageGlobal, widgetdomain.ZoneTop, widgetdomain.TypeNewArrivals, "Global Announcement", 1)
	w3, _ := widgetdomain.NewWidget("w3", widgetdomain.PageProduct, widgetdomain.ZoneBottom, widgetdomain.TypeNewArrivals, "Product Recommendations", 1)

	_ = wSrv.Create(context.Background(), w1)
	_ = wSrv.Create(context.Background(), w2)
	_ = wSrv.Create(context.Background(), w3)

	router, _ := newTestWidgetApp(t, wSrv)

	// List all
	req := httptest.NewRequest(http.MethodGet, "/admin/widgets", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "Home Hero Banner"))
	is.True(strings.Contains(body, "Global Announcement"))
	is.True(strings.Contains(body, "Product Recommendations"))

	// Filter by page=home
	reqFilter := httptest.NewRequest(http.MethodGet, "/admin/widgets?page=home", nil)
	setAdminSession(t, reqFilter)
	recFilter := httptest.NewRecorder()
	router.ServeHTTP(recFilter, reqFilter)

	is.Equal(recFilter.Code, http.StatusOK)
	bodyFilter := recFilter.Body.String()
	is.True(strings.Contains(bodyFilter, "Home Hero Banner"))
	is.True(!strings.Contains(bodyFilter, "Global Announcement"))
	is.True(!strings.Contains(bodyFilter, "Product Recommendations"))
}

func TestAdminWidgets_NewWidgetForm(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	router, _ := newTestWidgetApp(t, wSrv)

	req := httptest.NewRequest(http.MethodGet, "/admin/widgets/new", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()
	is.True(strings.Contains(body, "create widget"))
	is.True(strings.Contains(body, "banner_carousel"))
}

func TestAdminWidgets_CreateWidgets(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	router, _ := newTestWidgetApp(t, wSrv)

	// 1. Create banner_carousel
	form1 := url.Values{
		"page":          {"home"},
		"zone":          {"top"},
		"type":          {"banner_carousel"},
		"title":         {"Hero Carousel"},
		"display_order": {"1"},
		"is_active":     {"1"},
	}
	req1 := httptest.NewRequest(http.MethodPost, "/admin/widgets", strings.NewReader(form1.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req1)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	is.Equal(rec1.Code, http.StatusSeeOther)
	loc1 := rec1.Header().Get("Location")
	is.True(strings.HasPrefix(loc1, "/admin/widgets/"))
	is.True(strings.HasSuffix(loc1, "/edit"))

	// Verify widget stored
	all1, _ := wSrv.ListAll(context.Background())
	is.Equal(len(all1), 1)
	is.Equal(all1[0].Title, "Hero Carousel")
	is.Equal(all1[0].Type, widgetdomain.TypeBannerCarousel)

	// 2. Create new_arrivals
	form2 := url.Values{
		"page":          {"home"},
		"zone":          {"body"},
		"type":          {"new_arrivals"},
		"title":         {"Fresh Arrivals"},
		"display_order": {"2"},
		"is_active":     {"1"},
		"product_limit": {"12"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/widgets", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req2)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	is.Equal(rec2.Code, http.StatusSeeOther)
	all2, _ := wSrv.ListAll(context.Background())
	is.Equal(len(all2), 2)

	var naWidget widgetdomain.Widget
	for _, w := range all2 {
		if w.Type == widgetdomain.TypeNewArrivals {
			naWidget = w
		}
	}
	is.Equal(naWidget.Title, "Fresh Arrivals")
	var naCfg widgetdomain.NewArrivalsConfig
	err := json.Unmarshal(naWidget.Config, &naCfg)
	is.NoErr(err)
	is.Equal(naCfg.ProductLimit, 12)

	// 3. Create category_products
	form3 := url.Values{
		"page":          {"category"},
		"zone":          {"body"},
		"type":          {"category_products"},
		"title":         {"Featured Shoes"},
		"display_order": {"1"},
		"is_active":     {"1"},
		"category_id":   {"cat-shoes"},
		"product_limit": {"4"},
	}
	req3 := httptest.NewRequest(http.MethodPost, "/admin/widgets", strings.NewReader(form3.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, req3)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	is.Equal(rec3.Code, http.StatusSeeOther)
	all3, _ := wSrv.ListAll(context.Background())
	is.Equal(len(all3), 3)

	var cpWidget widgetdomain.Widget
	for _, w := range all3 {
		if w.Type == widgetdomain.TypeCategoryProducts {
			cpWidget = w
		}
	}
	is.Equal(cpWidget.Title, "Featured Shoes")
	var cpCfg widgetdomain.CategoryProductsConfig
	err = json.Unmarshal(cpWidget.Config, &cpCfg)
	is.NoErr(err)
	is.Equal(cpCfg.CategoryID, "cat-shoes")
	is.Equal(cpCfg.ProductLimit, 4)
}

func TestAdminWidgets_EditAndUpdateWidget(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	w1, _ := widgetdomain.NewWidget("w1", widgetdomain.PageHome, widgetdomain.ZoneBody, widgetdomain.TypeNewArrivals, "Old Title", 1)
	cfgBytes, _ := json.Marshal(widgetdomain.NewArrivalsConfig{ProductLimit: 8})
	w1.Config = cfgBytes
	_ = wSrv.Create(context.Background(), w1)

	router, _ := newTestWidgetApp(t, wSrv)

	// Get Edit form
	reqEdit := httptest.NewRequest(http.MethodGet, "/admin/widgets/w1/edit", nil)
	setAdminSession(t, reqEdit)
	recEdit := httptest.NewRecorder()
	router.ServeHTTP(recEdit, reqEdit)

	is.Equal(recEdit.Code, http.StatusOK)
	bodyEdit := recEdit.Body.String()
	is.True(strings.Contains(bodyEdit, "Old Title"))

	// Update Widget
	formUpdate := url.Values{
		"page":          {"home"},
		"zone":          {"bottom"},
		"title":         {"Updated Title"},
		"display_order": {"5"},
		"is_active":     {"1"},
		"product_limit": {"16"},
	}
	reqUpdate := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1", strings.NewReader(formUpdate.Encode()))
	reqUpdate.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqUpdate)
	recUpdate := httptest.NewRecorder()
	router.ServeHTTP(recUpdate, reqUpdate)

	is.Equal(recUpdate.Code, http.StatusSeeOther)
	is.Equal(recUpdate.Header().Get("Location"), "/admin/widgets/w1/edit")

	// Verify service state
	updated, err := wSrv.Get(context.Background(), "w1")
	is.NoErr(err)
	is.Equal(updated.Title, "Updated Title")
	is.Equal(updated.Zone, widgetdomain.ZoneBottom)
	is.Equal(updated.DisplayOrder, 5)

	var cfg widgetdomain.NewArrivalsConfig
	_ = json.Unmarshal(updated.Config, &cfg)
	is.Equal(cfg.ProductLimit, 16)
}

func TestAdminWidgets_ToggleActive(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	w1, _ := widgetdomain.NewWidget("w1", widgetdomain.PageHome, widgetdomain.ZoneTop, widgetdomain.TypeBannerCarousel, "Toggle Test", 1)
	w1.IsActive = true
	_ = wSrv.Create(context.Background(), w1)

	router, _ := newTestWidgetApp(t, wSrv)

	req := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1/toggle", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/widgets")

	updated, err := wSrv.Get(context.Background(), "w1")
	is.NoErr(err)
	is.Equal(updated.IsActive, false)
}

func TestAdminWidgets_DeleteWidget(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	w1, _ := widgetdomain.NewWidget("w1", widgetdomain.PageHome, widgetdomain.ZoneTop, widgetdomain.TypeBannerCarousel, "Delete Test", 1)
	_ = wSrv.Create(context.Background(), w1)

	router, _ := newTestWidgetApp(t, wSrv)

	req := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1/delete", nil)
	setAdminSession(t, req)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	is.Equal(rec.Code, http.StatusSeeOther)
	is.Equal(rec.Header().Get("Location"), "/admin/widgets")

	_, err := wSrv.Get(context.Background(), "w1")
	is.True(errors.Is(err, widgetdomain.ErrWidgetNotFound))
}

func TestAdminWidgets_AddAndDeleteSlides(t *testing.T) {
	is := is.New(t)
	setupTestEnvironment(t)

	wSrv := newMockWidgetService()
	w1, _ := widgetdomain.NewWidget("w1", widgetdomain.PageHome, widgetdomain.ZoneTop, widgetdomain.TypeBannerCarousel, "Carousel Widget", 1)
	_ = wSrv.Create(context.Background(), w1)

	router, _ := newTestWidgetApp(t, wSrv)

	// 1. Add slide with image URL
	formSlide1 := url.Values{
		"title":         {"Summer Sale"},
		"subtitle":      {"Up to 50% Off"},
		"image_url":     {"https://example.com/slide1.jpg"},
		"link_url":      {"/category/shoes"},
		"link_text":     {"Shop Shoes"},
		"promo_code":    {"SUMMER50"},
		"badge":         {"HOT"},
		"display_order": {"1"},
	}
	reqSlide1 := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1/slides", strings.NewReader(formSlide1.Encode()))
	reqSlide1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setAdminSession(t, reqSlide1)
	recSlide1 := httptest.NewRecorder()
	router.ServeHTTP(recSlide1, reqSlide1)

	is.Equal(recSlide1.Code, http.StatusSeeOther)
	is.Equal(recSlide1.Header().Get("Location"), "/admin/widgets/w1/edit")

	wUpdated1, _ := wSrv.Get(context.Background(), "w1")
	is.Equal(len(wUpdated1.Slides), 1)
	is.Equal(wUpdated1.Slides[0].Title, "Summer Sale")
	is.Equal(wUpdated1.Slides[0].ImageURL, "https://example.com/slide1.jpg")

	// 2. Add slide with image file upload (multipart)
	bodyBuf := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyBuf)
	_ = writer.WriteField("title", "File Upload Slide")
	_ = writer.WriteField("display_order", "2")

	// Dummy JPEG image content (a tiny 1x1 JPEG byte sequence or dummy header)
	jpegData := []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01\x01\x01\x00\x48\x00\x48\x00\x00\xFF\xD9")
	part, err := writer.CreateFormFile("image_file", "test_banner.jpg")
	is.NoErr(err)
	_, _ = part.Write(jpegData)
	_ = writer.Close()

	reqSlide2 := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1/slides", bodyBuf)
	reqSlide2.Header.Set("Content-Type", writer.FormDataContentType())
	setAdminSession(t, reqSlide2)
	recSlide2 := httptest.NewRecorder()
	router.ServeHTTP(recSlide2, reqSlide2)

	is.Equal(recSlide2.Code, http.StatusSeeOther)
	is.Equal(recSlide2.Header().Get("Location"), "/admin/widgets/w1/edit")

	wUpdated2, _ := wSrv.Get(context.Background(), "w1")
	is.Equal(len(wUpdated2.Slides), 2)
	is.Equal(wUpdated2.Slides[1].Title, "File Upload Slide")
	is.True(strings.HasPrefix(wUpdated2.Slides[1].ImageURL, "/uploads/"))

	// 3. Delete first slide
	slide1ID := wUpdated2.Slides[0].ID
	reqDelSlide := httptest.NewRequest(http.MethodPost, "/admin/widgets/w1/slides/"+slide1ID+"/delete", nil)
	setAdminSession(t, reqDelSlide)
	recDelSlide := httptest.NewRecorder()
	router.ServeHTTP(recDelSlide, reqDelSlide)

	is.Equal(recDelSlide.Code, http.StatusSeeOther)
	is.Equal(recDelSlide.Header().Get("Location"), "/admin/widgets/w1/edit")

	wUpdated3, _ := wSrv.Get(context.Background(), "w1")
	is.Equal(len(wUpdated3.Slides), 1)
	is.Equal(wUpdated3.Slides[0].Title, "File Upload Slide")
}
