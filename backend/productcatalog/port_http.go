package productcatalog

import (
	"errors"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

type httpPort struct {
	serv ProductService
}

func newPortHTTP(appServ ProductService) httpPort {
	return httpPort{
		serv: appServ,
	}
}

type httpProduct struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       price  `json:"price"`
	Thumbnail   string `json:"thumbnail"`
}

type price struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// @Router       /products [get]
// @Success      200  {object}  []httpProduct
// @Accept       json
// @Produce      json
// @Failure      500  {object}  https.ErrorResponse
// @Failure      404  {object}  https.ErrorResponse
func (h httpPort) AllProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.serv.AllProducts(r.Context())
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all products")
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	resp := toAllProductsResponse(products)
	https.OK(w, resp)
}

func (h httpPort) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := h.serv.Find(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			https.NotFound(w, "product-not-found", "product does not exists")
		} else {
			https.InternalError(w, "internal-error", "cannot get list of all products")
		}

		log.Printf("cannot get list of all products: %s", err)
		return
	}

	https.OK(w, productsToResponse(product))
}

func toAllProductsResponse(products []Product) []httpProduct {
	resp := []httpProduct{}

	for _, prod := range products {
		resp = append(resp, productsToResponse(prod))
	}

	return resp
}

func productsToResponse(prod Product) httpProduct {
	return httpProduct{
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
