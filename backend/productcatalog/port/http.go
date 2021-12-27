package port

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/gorilla/mux"
)

type HTTP struct {
	serv app.ProductService
}

func NewHTTP(productStorage app.ProductStorage) HTTP {
	return HTTP{
		serv: app.NewProductService(productStorage),
	}
}

type AllProductsResponse struct {
	Products []product
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
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	body, err := json.Marshal(toAllProductsResponse(products))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot marshal products: %s", err)
		return
	}

	cors(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h HTTP) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := h.serv.Find(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	body, err := json.Marshal(productsToResponse(product))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("cannot marshal products: %s", err)
		return
	}

	cors(w)
	w.Header().Add("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func toAllProductsResponse(products []domain.Product) AllProductsResponse {
	resp := AllProductsResponse{}

	for _, prod := range products {
		resp.Products = append(resp.Products, productsToResponse(prod))
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

func cors(w http.ResponseWriter) {
	var allowedHeaders = "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization,X-CSRF-Token"
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
	w.Header().Set("Access-Control-Expose-Headers", "Authorization")
}
