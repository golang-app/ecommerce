package port

import (
	"errors"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

type HTTP struct {
	serv app.ProductService
}

func NewHTTP(appServ app.ProductService) HTTP {
	return HTTP{
		serv: appServ,
	}
}

type product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       price  `json:"price"`
	Thumbnail   string `json:"thumbnail"`
}

type price struct {
	Currency string  `json:"currency"`
	Amount   float32 `json:"amount"`
}

func (h HTTP) AllProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.serv.AllProducts(r.Context())
	if err != nil {
		https.InternalError(w, "cannot get list of all products:")
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	resp := toAllProductsResponse(products)
	https.OK(w, resp)
}

func (h HTTP) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := h.serv.Find(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			https.NotFound(w, "product does not exists")
		} else {
			https.InternalError(w, "cannot get list of all products")
		}

		log.Printf("cannot get list of all products: %s", err)
		return
	}

	https.OK(w, productsToResponse(product))
}

func toAllProductsResponse(products []domain.Product) []product {
	resp := []product{}

	for _, prod := range products {
		resp = append(resp, productsToResponse(prod))
	}

	return resp
}

func productsToResponse(prod domain.Product) product {
	return product{
		ID:          string(prod.ID()),
		Name:        prod.Name(),
		Description: prod.Description(),
		Price: price{
			Amount:   prod.Price().Amount(),
			Currency: prod.Price().Currency(),
		},
		Thumbnail: prod.Thumbnail(),
	}
}
