package layout

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

var (
	key   = []byte("go-ecommerce")
	store = sessions.NewCookieStore(key)
)

type httpHandler struct {
	cartSrv    cartService
	catalogSrv catalogService
	authSrv    authService
}

func (handler httpHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "home", nil)
}

func (m boundedContext) MuxRegister(r *mux.Router) {
	r.HandleFunc("/", m.handler.HomePage)

	r.HandleFunc("/cart/{cartID}", observability.LoggerMiddleware(https.WrapPanic(m.handler.AddToCart), m.logger)).Methods("POST")
	r.HandleFunc("/cart/budge", observability.LoggerMiddleware(https.WrapPanic(m.handler.Budge), m.logger)).Methods("GET", "OPTIONS")

	r.HandleFunc("/api/v1/products", m.handler.AllProducts)
	r.HandleFunc("/product/{productID}", m.handler.Product)

	r.HandleFunc("/auth/login", m.handler.Login).Methods("GET")
	r.HandleFunc("/auth/login", m.handler.HandleLogin).Methods("POST")
	r.HandleFunc("/auth/logout", m.handler.HandleLogout).Methods("GET")
	r.HandleFunc("/auth/register", m.handler.Register).Methods("GET")
	r.HandleFunc("/auth/register", m.handler.HandleRegister).Methods("POST")
	r.HandleFunc("/auth/menuIcon", observability.LoggerMiddleware(https.WrapPanic(m.handler.AuthMenuItem), m.logger)).Methods("GET", "OPTIONS")
}

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.ParseGlob("layout/tmpl/*.gohtml"))
	tmpl = template.Must(tmpl.ParseGlob("layout/tmpl/productCatalog/*.gohtml"))
	tmpl = template.Must(tmpl.ParseGlob("layout/tmpl/cart/*.gohtml"))
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
