package layout

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

func (handler httpHandler) AllProducts(w http.ResponseWriter, r *http.Request) {
	products, err := handler.catalogSrv.AllProducts(r.Context())
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all products")
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	resp := map[string]any{
		"Products": products,
	}

	files := []string{
		"./layout/tmpl/productCatalog/allProducts.gohtml",
	}

	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"add": func(a, b string) float64 {
			return 666
		},
	}).ParseFiles(files...))
	err = ts.ExecuteTemplate(w, "allProducts.gohtml", resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (handler httpHandler) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)

	session, _ := store.Get(r, "ecommerce")

	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			session.AddFlash("Product does not exists", "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		session.AddFlash("cannot get list of all products", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	resp := map[string]any{
		"Product": product,
	}
	handler.renderTemplate(w, r, "productCatalog/show", resp)
}
