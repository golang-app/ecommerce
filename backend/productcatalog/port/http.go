package port

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
)

type HTTP struct {
	serv app.ProductService
}

type AllProductsResponse struct {
	Products []product
}

type product struct {
	ID          string
	Name        string
	Description string
	Price       price
	Thumbnail   string
}

type price struct {
	Currency string
	Amount   float32
}

func (h HTTP) AllProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.serv.AllProducts(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	body, err := json.Marshal(products)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot marshal products: %s", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Add("content-type", "application/json")
	_, _ = w.Write(body)
}
