package layout

import (
	"errors"
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

	err = tmpl.ExecuteTemplate(w, "allProducts.gohtml", resp)
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
			session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		session.AddFlash("cannot get list of all products", "error")
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	resp := map[string]any{
		"Product": product,
	}
	handler.renderTemplate(w, r, "productCatalog/show", resp)
}
