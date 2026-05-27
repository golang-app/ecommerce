package layout

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var (
	key   = []byte("go-ecommerce")
	store = newCookieStore(key)
)

// newCookieStore returns a CookieStore whose Options work over plain HTTP
// (localhost / docker compose) so the demo runs without TLS. gorilla
// sessions defaults to Secure + SameSite=None which makes the cookie
// invisible to non-HTTPS clients. Flip Secure to true when serving over
// HTTPS for real.
func newCookieStore(key []byte) *sessions.CookieStore {
	s := sessions.NewCookieStore(key)
	s.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   false,
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
}

func (handler httpHandler) HomePage(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("cannot get categories: %s", err)
		categories = nil
	}

	facets, err := handler.catalogSrv.Facets(r.Context(), activeCategory)
	if err != nil {
		log.Printf("cannot get facets for %q: %s", activeCategory, err)
		facets = nil
	}

	handler.renderTemplate(w, r, "home", map[string]any{
		"Categories":     categories,
		"Facets":         facets,
		"ActiveCategory": activeCategory,
	})
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.PathPrefix("/static/").Handler(StaticHandler())
	r.HandleFunc("/", m.handler.HomePage)
	r.HandleFunc("/category/{slug}", observability.HTTPWrap(m.handler.CategoryPage, m.logger)).Methods("GET")

	r.HandleFunc("/cart", observability.HTTPWrap(m.handler.Cart, m.logger)).Methods("GET")
	r.HandleFunc("/cart/{variantID}", observability.HTTPWrap(m.handler.AddToCart, m.logger)).Methods("POST")
	r.HandleFunc("/cart/budge", observability.HTTPWrap(m.handler.Budge, m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/products", observability.HTTPWrap(m.handler.AllProducts, m.logger))
	r.HandleFunc("/product/{productID}", observability.HTTPWrap(m.handler.Product, m.logger)).Methods("GET")
	r.HandleFunc("/product/{productID}/variant", observability.HTTPWrap(m.handler.ProductVariant, m.logger)).Methods("GET")

	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.Login, m.logger)).Methods("GET")
	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.HandleLogin, m.logger)).Methods("POST")
	r.HandleFunc("/auth/logout", observability.HTTPWrap(m.handler.HandleLogout, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.Register, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.HandleRegister, m.logger)).Methods("POST")
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

	// Admin panel. Later phases register the /admin/products, /admin/categories,
	// /admin/attributes and /admin/orders handlers here, all behind requireAdmin.
	r.HandleFunc("/admin", observability.HTTPWrap(m.handler.AdminDashboard, m.logger)).Methods("GET")
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

	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
	}).ParseFiles(files...))

	session, _ := store.Get(r, "ecommerce")
	data["FlashInfo"] = session.Flashes()
	data["FlashError"] = session.Flashes("error")
	data["AuthMenuItem"] = renderPartial(w, r, http.HandlerFunc(handler.AuthMenuItem))
	data["LoggedIn"] = handler.currentCustomerID(r) != ""
	data["IsAdmin"] = handler.isAdmin(r)
	err := session.Save(r, w)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
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
