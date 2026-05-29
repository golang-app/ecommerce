package layout

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
	"github.com/bkielbasa/go-ecommerce/backend/internal/imagestore"
	"github.com/bkielbasa/go-ecommerce/backend/internal/mailer"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
)

// store is the process-wide session cookie store. It is initialised once by
// layout.New() (which reads SESSION_SECRET and COOKIE_SECURE from config) and
// reused by every handler in this package. Keeping it package-level keeps the
// existing `store.Get(r, "ecommerce")` call-sites unchanged; there is only
// one layout boundedContext per process so a single shared store is fine.
var store *sessions.CookieStore

// newCookieStore returns a CookieStore whose Options work over plain HTTP
// (localhost / docker compose) when secure=false; flip secure=true for
// HTTPS deployments (COOKIE_SECURE=true). HttpOnly and SameSite=Lax are
// always on.
func newCookieStore(key []byte, secure bool) *sessions.CookieStore {
	s := sessions.NewCookieStore(key)
	s.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
	return s
}

type httpHandler struct {
	cartSrv     cartService
	catalogSrv  catalogService
	authSrv     authService
	checkoutSrv checkoutCommands
	checkoutQry checkoutQueries
	shipSrv     shippingService
	reviewsSrv  reviewsService
	wishlistSrv wishlistService
	promoSrv    promoService
	searchSrv   searchService
	storeSrv    storeService
	imageStore  imagestore.Store
	mailer      mailer.Mailer
	baseURL     string
	// rates is the static, operator-configured FX table. It is shared
	// across every render through the `money` template helper. The
	// active currency comes from the request-bound store, but the
	// rates table itself is shared by every store (it just knows how
	// to convert FROM USD TO each supported display currency).
	rates  fx.Rates
	logger logrus.FieldLogger
}

// HomePage renders the storefront landing page: a "new arrivals" grid of the
// newest products plus a link through to the full shop.
func (handler httpHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	newest, err := handler.catalogSrv.Newest(r.Context(), 8)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot get newest products")
		newest = nil
	}
	handler.renderTemplate(w, r, "home", map[string]any{
		"Newest": newest,
	})
}

// ShopPage renders the full filterable catalog (all categories).
func (handler httpHandler) ShopPage(w http.ResponseWriter, r *http.Request) {
	handler.renderProductsPage(w, r, "")
}

// CategoryPage renders the full products page scoped to a single category
// (by slug). An unknown slug still renders (with an empty grid) rather than
// erroring.
func (handler httpHandler) CategoryPage(w http.ResponseWriter, r *http.Request) {
	slug := mux.Vars(r)["slug"]
	handler.renderProductsPage(w, r, slug)
}

// renderProductsPage renders the full products page (left filter rail + grid
// container) for the given category scope ("" means "all"). The grid itself is
// lazy-loaded over HTMX from /api/v1/products.
func (handler httpHandler) renderProductsPage(w http.ResponseWriter, r *http.Request, activeCategory string) {
	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot get categories")
		categories = nil
	}

	facets, err := handler.catalogSrv.Facets(r.Context(), activeCategory)
	if err != nil {
		handler.logger.WithError(err).WithField("category", activeCategory).Warn("cannot get facets")
		facets = nil
	}

	handler.renderTemplate(w, r, "productCatalog/catalog", map[string]any{
		"Categories":     categories,
		"Facets":         facets,
		"ActiveCategory": activeCategory,
		"Search":         "",
	})
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.PathPrefix("/static/").Handler(StaticHandler())
	// User-uploaded images live on disk (see internal/imagestore) and are
	// served from m.uploadsDir under the /uploads/ URL prefix.
	r.PathPrefix("/uploads/").Handler(imagestore.Serve(m.uploadsDir))
	// SEO endpoints. Registered before the "/" catch-all so they are not
	// shadowed by the home page route. Both are public (no admin gate).
	r.HandleFunc("/robots.txt", observability.HTTPWrap(m.handler.Robots, m.logger)).Methods("GET")
	r.HandleFunc("/sitemap.xml", observability.HTTPWrap(m.handler.Sitemap, m.logger)).Methods("GET")
	// Public search page. Registered before the "/" catch-all.
	r.HandleFunc("/search", observability.HTTPWrap(m.handler.SearchPage, m.logger)).Methods("GET")
	r.HandleFunc("/", m.handler.HomePage)
	r.HandleFunc("/products", observability.HTTPWrap(m.handler.ShopPage, m.logger)).Methods("GET")
	r.HandleFunc("/category/{slug}", observability.HTTPWrap(m.handler.CategoryPage, m.logger)).Methods("GET")
	// Footer-driven store switcher. Lists every configured store with
	// a link that drops the visitor onto the same path on the other
	// store's host. Replaces the previous per-customer currency picker
	// (which has been removed) — the display currency now follows the
	// active store, not a per-shopper preference.
	r.HandleFunc("/stores", observability.HTTPWrap(m.handler.StoresPage, m.logger)).Methods("GET")

	r.HandleFunc("/cart", observability.HTTPWrap(m.handler.Cart, m.logger)).Methods("GET")
	r.HandleFunc("/cart/{variantID}", observability.HTTPWrap(m.handler.AddToCart, m.logger)).Methods("POST")
	r.HandleFunc("/cart/budge", observability.HTTPWrap(m.handler.Budge, m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/products", observability.HTTPWrap(m.handler.AllProducts, m.logger))
	r.HandleFunc("/product/{productID}", observability.HTTPWrap(m.handler.Product, m.logger)).Methods("GET")
	r.HandleFunc("/product/{productID}/variant", observability.HTTPWrap(m.handler.ProductVariant, m.logger)).Methods("GET")
	r.HandleFunc("/product/{productID}/review", observability.HTTPWrap(m.handler.SubmitReview, m.logger)).Methods("POST")

	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.Login, m.logger)).Methods("GET")
	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.HandleLogin, m.logger)).Methods("POST")
	r.HandleFunc("/auth/logout", observability.HTTPWrap(m.handler.HandleLogout, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.Register, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.HandleRegister, m.logger)).Methods("POST")
	r.HandleFunc("/auth/change-password", observability.HTTPWrap(m.handler.ChangePasswordPage, m.logger)).Methods("GET")
	r.HandleFunc("/auth/change-password", observability.HTTPWrap(m.handler.HandleChangePassword, m.logger)).Methods("POST")
	r.HandleFunc("/auth/forgot", observability.HTTPWrap(m.handler.ForgotPasswordPage, m.logger)).Methods("GET")
	r.HandleFunc("/auth/forgot", observability.HTTPWrap(m.handler.HandleForgotPassword, m.logger)).Methods("POST")
	r.HandleFunc("/auth/reset", observability.HTTPWrap(m.handler.ResetPasswordPage, m.logger)).Methods("GET")
	r.HandleFunc("/auth/reset", observability.HTTPWrap(m.handler.HandleResetPassword, m.logger)).Methods("POST")
	r.HandleFunc("/auth/menuIcon", observability.HTTPWrap(m.handler.AuthMenuItem, m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/checkout", observability.HTTPWrap(m.handler.Checkout, m.logger)).Methods("GET")
	r.HandleFunc("/checkout", observability.HTTPWrap(m.handler.PlaceOrder, m.logger)).Methods("POST")
	r.HandleFunc("/orders", observability.HTTPWrap(m.handler.Orders, m.logger)).Methods("GET")
	r.HandleFunc("/order/{orderID}", observability.HTTPWrap(m.handler.Order, m.logger)).Methods("GET")
	r.HandleFunc("/order/{orderID}/cancel", observability.HTTPWrap(m.handler.CancelOrder, m.logger)).Methods("POST")

	r.HandleFunc("/account", observability.HTTPWrap(m.handler.AccountOverview, m.logger)).Methods("GET")
	r.HandleFunc("/account/orders", observability.HTTPWrap(m.handler.AccountOrders, m.logger)).Methods("GET")
	r.HandleFunc("/account/addresses", observability.HTTPWrap(m.handler.AccountAddresses, m.logger)).Methods("GET")
	r.HandleFunc("/account/addresses", observability.HTTPWrap(m.handler.AccountAddAddress, m.logger)).Methods("POST")
	r.HandleFunc("/account/addresses/{id}/edit", observability.HTTPWrap(m.handler.AccountEditAddressForm, m.logger)).Methods("GET")
	r.HandleFunc("/account/addresses/{id}", observability.HTTPWrap(m.handler.AccountUpdateAddress, m.logger)).Methods("POST")
	r.HandleFunc("/account/addresses/{id}/delete", observability.HTTPWrap(m.handler.AccountDeleteAddress, m.logger)).Methods("POST")
	r.HandleFunc("/account/addresses/{id}/default", observability.HTTPWrap(m.handler.AccountSetDefaultAddress, m.logger)).Methods("POST")
	r.HandleFunc("/account/details", observability.HTTPWrap(m.handler.AccountDetails, m.logger)).Methods("GET")
	r.HandleFunc("/account/details/password", observability.HTTPWrap(m.handler.AccountChangePassword, m.logger)).Methods("POST")
	r.HandleFunc("/account/wishlist", observability.HTTPWrap(m.handler.AccountWishlist, m.logger)).Methods("GET")

	r.HandleFunc("/wishlist/{variantID}/toggle", observability.HTTPWrap(m.handler.WishlistToggle, m.logger)).Methods("POST")

	// Admin panel. Later phases register the /admin/products, /admin/categories,
	// /admin/attributes and /admin/orders handlers here, all behind requireAdmin.
	r.HandleFunc("/admin", observability.HTTPWrap(m.handler.AdminDashboard, m.logger)).Methods("GET")

	r.HandleFunc("/admin/products", observability.HTTPWrap(m.handler.AdminProducts, m.logger)).Methods("GET")
	r.HandleFunc("/admin/products", observability.HTTPWrap(m.handler.AdminCreateProduct, m.logger)).Methods("POST")
	// Variant-product routes: literal/longer paths first so they are not
	// captured by the /admin/products/{id} catch-all below.
	r.HandleFunc("/admin/products/new-variant", observability.HTTPWrap(m.handler.AdminNewVariantProductForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/products/new-variant", observability.HTTPWrap(m.handler.AdminCreateVariantProduct, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/new", observability.HTTPWrap(m.handler.AdminNewProductForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/products/{id}/variants/{variantID}/delete", observability.HTTPWrap(m.handler.AdminDeleteVariant, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/variants/{variantID}", observability.HTTPWrap(m.handler.AdminUpdateVariant, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/variants", observability.HTTPWrap(m.handler.AdminAddVariant, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/options/update", observability.HTTPWrap(m.handler.AdminUpdateOptionType, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/options/delete", observability.HTTPWrap(m.handler.AdminDeleteOptionType, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/options", observability.HTTPWrap(m.handler.AdminAddOptionType, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/edit", observability.HTTPWrap(m.handler.AdminEditProductForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/products/{id}/stock", observability.HTTPWrap(m.handler.AdminUpdateProductStock, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/categories", observability.HTTPWrap(m.handler.AdminUpdateProductCategories, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/attributes", observability.HTTPWrap(m.handler.AdminUpdateProductAttributes, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/attribute-set", observability.HTTPWrap(m.handler.AdminUpdateProductAttributeSet, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteProduct, m.logger)).Methods("POST")
	r.HandleFunc("/admin/products/{id}", observability.HTTPWrap(m.handler.AdminUpdateProduct, m.logger)).Methods("POST")

	r.HandleFunc("/admin/categories", observability.HTTPWrap(m.handler.AdminCategories, m.logger)).Methods("GET")
	r.HandleFunc("/admin/categories", observability.HTTPWrap(m.handler.AdminCreateCategory, m.logger)).Methods("POST")
	r.HandleFunc("/admin/categories/{id}/edit", observability.HTTPWrap(m.handler.AdminEditCategoryForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/categories/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteCategory, m.logger)).Methods("POST")
	r.HandleFunc("/admin/categories/{id}", observability.HTTPWrap(m.handler.AdminUpdateCategory, m.logger)).Methods("POST")

	r.HandleFunc("/admin/attributes", observability.HTTPWrap(m.handler.AdminAttributes, m.logger)).Methods("GET")
	r.HandleFunc("/admin/attributes", observability.HTTPWrap(m.handler.AdminCreateAttribute, m.logger)).Methods("POST")
	r.HandleFunc("/admin/attributes/{id}/edit", observability.HTTPWrap(m.handler.AdminEditAttributeForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/attributes/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteAttribute, m.logger)).Methods("POST")
	r.HandleFunc("/admin/attributes/{id}", observability.HTTPWrap(m.handler.AdminUpdateAttribute, m.logger)).Methods("POST")

	r.HandleFunc("/admin/attribute-sets", observability.HTTPWrap(m.handler.AdminAttributeSets, m.logger)).Methods("GET")
	r.HandleFunc("/admin/attribute-sets", observability.HTTPWrap(m.handler.AdminCreateAttributeSet, m.logger)).Methods("POST")
	r.HandleFunc("/admin/attribute-sets/new", observability.HTTPWrap(m.handler.AdminNewAttributeSetForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/attribute-sets/{id}/edit", observability.HTTPWrap(m.handler.AdminEditAttributeSetForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/attribute-sets/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteAttributeSet, m.logger)).Methods("POST")
	r.HandleFunc("/admin/attribute-sets/{id}", observability.HTTPWrap(m.handler.AdminUpdateAttributeSet, m.logger)).Methods("POST")

	r.HandleFunc("/admin/orders", observability.HTTPWrap(m.handler.AdminOrders, m.logger)).Methods("GET")
	// Specific fulfillment sub-paths must be registered before the
	// /admin/orders/{orderID} catch-all so they don't get shadowed.
	r.HandleFunc("/admin/orders/{orderID}/cancel", observability.HTTPWrap(m.handler.AdminCancelOrder, m.logger)).Methods("POST")
	r.HandleFunc("/admin/orders/{orderID}/ship", observability.HTTPWrap(m.handler.AdminShipOrder, m.logger)).Methods("POST")
	r.HandleFunc("/admin/orders/{orderID}/deliver", observability.HTTPWrap(m.handler.AdminDeliverOrder, m.logger)).Methods("POST")
	r.HandleFunc("/admin/orders/{orderID}/refund", observability.HTTPWrap(m.handler.AdminRefundOrder, m.logger)).Methods("POST")
	r.HandleFunc("/admin/orders/{orderID}", observability.HTTPWrap(m.handler.AdminOrderDetail, m.logger)).Methods("GET")

	r.HandleFunc("/admin/inventory", observability.HTTPWrap(m.handler.AdminInventory, m.logger)).Methods("GET")

	r.HandleFunc("/admin/reviews", observability.HTTPWrap(m.handler.AdminReviews, m.logger)).Methods("GET")
	r.HandleFunc("/admin/reviews/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteReview, m.logger)).Methods("POST")

	// Promo codes admin: list + inline create on /admin/promo-codes; the
	// /{code}/edit and /{code}/delete sub-paths are registered before the
	// /{code} catch-all so the latter doesn't shadow them.
	r.HandleFunc("/admin/promo-codes", observability.HTTPWrap(m.handler.AdminPromoCodes, m.logger)).Methods("GET")
	r.HandleFunc("/admin/promo-codes", observability.HTTPWrap(m.handler.AdminCreatePromoCode, m.logger)).Methods("POST")
	r.HandleFunc("/admin/promo-codes/{code}/edit", observability.HTTPWrap(m.handler.AdminEditPromoCodeForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/promo-codes/{code}/delete", observability.HTTPWrap(m.handler.AdminDeletePromoCode, m.logger)).Methods("POST")
	r.HandleFunc("/admin/promo-codes/{code}", observability.HTTPWrap(m.handler.AdminUpdatePromoCode, m.logger)).Methods("POST")

	// Stores admin: list + inline create on /admin/stores; the
	// /{id}/edit and /{id}/delete sub-paths are registered before the
	// /{id} catch-all so the latter doesn't shadow them.
	r.HandleFunc("/admin/stores", observability.HTTPWrap(m.handler.AdminStores, m.logger)).Methods("GET")
	r.HandleFunc("/admin/stores", observability.HTTPWrap(m.handler.AdminCreateStore, m.logger)).Methods("POST")
	r.HandleFunc("/admin/stores/{id}/edit", observability.HTTPWrap(m.handler.AdminEditStoreForm, m.logger)).Methods("GET")
	r.HandleFunc("/admin/stores/{id}/delete", observability.HTTPWrap(m.handler.AdminDeleteStore, m.logger)).Methods("POST")
	r.HandleFunc("/admin/stores/{id}", observability.HTTPWrap(m.handler.AdminUpdateStore, m.logger)).Methods("POST")
}

func (handler httpHandler) renderTemplate(w http.ResponseWriter, r *http.Request, templateName string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	files := []string{
		"./layout/tmpl/layout.gohtml",
		"./layout/tmpl/" + templateName + ".gohtml",
	}
	// Shared partials (e.g. the account sidebar) are available to every page.
	partials, _ := filepath.Glob("./layout/tmpl/partials/*.gohtml")
	files = append(files, partials...)

	// Active display currency is captured once per request and closed over
	// by the `money` helper below; that way every {{ money .X }} call
	// inside a single render uses the same currency without the template
	// having to thread it through every partial.
	currency := handler.currentCurrency(r)

	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"add":      func(a, b int) int { return a + b },
		"truncate": truncateForMeta,
		"dict":     templateDict,
		// money converts a minor-unit amount stored in the default
		// currency (USD) to the customer's active display currency and
		// formats it as "X.YY CCY". The amount stays in USD inside the
		// database; only the rendered HTML changes.
		"money": moneyFunc(handler.rates, currency),
	}).ParseFiles(files...))

	session, _ := store.Get(r, "ecommerce")
	// CSRFToken is injected into every page so:
	//   * hidden <input name="csrf_token"> fields in <form method="post"> get a
	//     fresh value on each render, and
	//   * the htmx:configRequest snippet in layout.gohtml can stamp the same
	//     value onto the X-CSRF-Token header for HTMX-driven POSTs.
	// issueCSRFToken shares the request-scoped gorilla/sessions registry, so
	// it mutates the same `session` pointer; the flash reads and the
	// session.Save below therefore see the just-minted token.
	csrfToken, err := issueCSRFToken(r, w)
	if err != nil {
		handler.logger.WithError(err).Error("cannot issue CSRF token")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data["CSRFToken"] = csrfToken
	data["FlashInfo"] = session.Flashes()
	data["FlashError"] = session.Flashes("error")
	data["AuthMenuItem"] = renderPartial(w, r, http.HandlerFunc(handler.AuthMenuItem))
	data["LoggedIn"] = handler.currentCustomerID(r) != ""
	data["IsAdmin"] = handler.isAdmin(r)
	// SEO helpers consumed by the base template's title/og/canonical blocks.
	// SiteName is the brand suffix in <title> ("Foo · GoCommerce"); CanonicalURL
	// is the absolute URL for the current request (scheme + host + path), used
	// for <link rel=canonical> and og:url.
	data["SiteName"] = "GoCommerce"
	data["CanonicalURL"] = requestBaseURL(r) + r.URL.Path
	// NavCategories lets the storefront header list category links on every page.
	navCategories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		handler.logger.WithError(err).Warn("cannot get nav categories")
		navCategories = nil
	}
	data["NavCategories"] = navCategories
	// Currency is the active display currency for the request, sourced
	// from the request-bound Store (see http_store.go currentCurrency).
	// ActiveStore is the resolved Store value object so templates can
	// reference its name/slug; Stores is the full list the footer
	// switcher renders. Admin renders do NOT set these (they always
	// show USD); see renderAdminTemplate.
	data["Currency"] = currency
	data["ActiveStore"] = storeFromCtx(r)
	if handler.storeSrv != nil {
		stores, sErr := handler.storeSrv.ListAll(r.Context())
		if sErr != nil {
			handler.logger.WithError(sErr).Warn("cannot list stores for footer switcher")
			stores = nil
		}
		data["Stores"] = stores
	} else {
		data["Stores"] = nil
	}
	// SearchQuery is the value the header search input shows. It defaults to
	// "" so every page renders the box; the search/catalog pages override it
	// from the URL `q`.
	if _, ok := data["SearchQuery"]; !ok {
		data["SearchQuery"] = ""
	}
	err = session.Save(r, w)
	if err != nil {
		handler.logger.WithError(err).Error("cannot save session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		handler.logger.WithError(err).Error("cannot execute template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderAdminTemplate mirrors renderTemplate but renders inside the dedicated
// admin shell (tmpl/admin/layout.gohtml, which defines "adminbase") instead of
// the storefront base. It still pulls in the shared partials glob (where the
// admin-nav partial lives) and the page template (which defines "main"), but it
// does not need AuthMenuItem/LoggedIn/IsAdmin — the admin shell exposes the
// signed-in admin via data["AdminEmail"].
func (handler httpHandler) renderAdminTemplate(w http.ResponseWriter, r *http.Request, templateName string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	files := []string{
		"./layout/tmpl/admin/layout.gohtml",
		"./layout/tmpl/" + templateName + ".gohtml",
	}
	partials, _ := filepath.Glob("./layout/tmpl/partials/*.gohtml")
	files = append(files, partials...)

	// The partials glob pulls in storefront partials (e.g. variant-box)
	// that reference the `money` helper. Admin pages never invoke those
	// partials, but html/template still needs every function mentioned
	// in any parsed template to be registered at parse time — so we
	// install a USD-fixed `money` here. The admin section is deliberately
	// pinned to the storage currency: operators see the amount that will
	// actually be charged, not a converted display value.
	adminMoney := moneyFunc(handler.rates, handler.rates.Default())
	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"add":   func(a, b int) int { return a + b },
		"join":  func(sep string, items []string) string { return strings.Join(items, sep) },
		"dict":  templateDict,
		"money": adminMoney,
	}).ParseFiles(files...))

	session, _ := store.Get(r, "ecommerce")
	// Same CSRF/htmx contract as renderTemplate — the admin shell also
	// embeds the configRequest script that stamps X-CSRF-Token on every
	// HTMX request, and admin templates emit a hidden csrf_token input.
	csrfToken, err := issueCSRFToken(r, w)
	if err != nil {
		handler.logger.WithError(err).Error("cannot issue CSRF token")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	data["CSRFToken"] = csrfToken
	data["FlashInfo"] = session.Flashes()
	data["FlashError"] = session.Flashes("error")
	data["AdminEmail"] = handler.currentCustomerID(r)
	err = session.Save(r, w)
	if err != nil {
		handler.logger.WithError(err).Error("cannot save session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	err = ts.ExecuteTemplate(w, "adminbase", data)
	if err != nil {
		handler.logger.WithError(err).Error("cannot execute admin template")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// templateDict builds a map[string]any from alternating key/value
// arguments inside a template (`dict "VariantID" .Variant.ID ...`). This
// lets a partial be invoked with a synthesised dot, which is how the
// "wishlist-button" partial gets its fields when it's embedded inside the
// variant-box (the parent dot there is the page's data map, not a flat
// {VariantID, InWishlist, LoggedIn} triple).
func templateDict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: expected an even number of arguments, got %d", len(pairs))
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is not a string (%T)", i, pairs[i])
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}

func renderPartial(w http.ResponseWriter, r *http.Request, h http.HandlerFunc) string {
	buf := buffer{buf: bytes.NewBufferString("")}
	h.ServeHTTP(http.ResponseWriter(&buf), r)

	return buf.buf.String()
}

type buffer struct {
	buf *bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

func (b *buffer) Header() http.Header {
	return http.Header{}
}

func (b *buffer) WriteHeader(statusCode int) {
}
