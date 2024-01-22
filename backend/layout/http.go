package layout

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"

	authDomain "github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	"github.com/bkielbasa/go-ecommerce/backend/shipment"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var (
	key   = []byte("go-ecommerce")
	store = sessions.NewCookieStore(key)
)

type catalogService interface {
	AllProducts(ctx context.Context) ([]productcatalog.Product, error)
	Find(ctx context.Context, id string) (productcatalog.Product, error)
}

type cartService interface {
	AddToCart(ctx context.Context, sessID string, productID string, qty int) error
	Get(ctx context.Context, sessID string) (*domain.Cart, error)
}

type authService interface {
	Login(ctx context.Context, username string, password string) (*authDomain.Session, error)
	Logout(ctx context.Context, seesionID string) error
	CreateNewCustomer(ctx context.Context, email, password string) error
	FindByToken(ctx context.Context, sessToken string) (*authDomain.Session, error)
}

type shipmentService interface {
	List(ctx context.Context, customerID string) ([]shipment.Address, error)
}

type httpHandler struct {
	cartSrv     cartService
	catalogSrv  catalogService
	authSrv     authService
	shipmentSrv shipmentService
}

func (handler httpHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "home", nil)
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/", m.handler.HomePage)

	r.HandleFunc("/cart", observability.HTTPWrap(m.handler.Cart, m.logger)).Methods("GET")
	r.HandleFunc("/cart/{cartID}", observability.HTTPWrap(m.handler.AddToCart, m.logger)).Methods("POST")
	r.HandleFunc("/cart/budge", observability.HTTPWrap(m.handler.Budge, m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/products", observability.HTTPWrap(m.handler.AllProducts, m.logger))
	r.HandleFunc("/product/{productID}", observability.HTTPWrap(m.handler.Product, m.logger))

	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.Login, m.logger)).Methods("GET")
	r.HandleFunc("/auth/login", observability.HTTPWrap(m.handler.HandleLogin, m.logger)).Methods("POST")
	r.HandleFunc("/auth/logout", observability.HTTPWrap(m.handler.HandleLogout, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.Register, m.logger)).Methods("GET")
	r.HandleFunc("/auth/register", observability.HTTPWrap(m.handler.HandleRegister, m.logger)).Methods("POST")
	r.HandleFunc("/auth/menuIcon", observability.HTTPWrap(m.handler.AuthMenuItem, m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/shipment/address", observability.HTTPWrap(m.handler.AllShipmentAddresses, m.logger))
	r.HandleFunc("/shipment/address/new", observability.HTTPWrap(m.handler.NewShipment, m.logger))
}

func (handler httpHandler) renderTemplate(w http.ResponseWriter, r *http.Request, templateName string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	files := []string{
		"./layout/tmpl/layout.gohtml",
		"./layout/tmpl/" + templateName + ".gohtml",
	}

	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
	}).ParseFiles(files...))

	session, _ := store.Get(r, "ecommerce")
	data["FlashInfo"] = session.Flashes()
	data["FlashError"] = session.Flashes("error")
	data["AuthMenuItem"] = renderPartial(w, r, http.HandlerFunc(handler.AuthMenuItem))
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
