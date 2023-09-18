package productcatalog

import (
	_ "embed"
	"errors"
	"html/template"
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

//go:embed tmpl/allproducts.gohtml
var allproductsTmpl string

func (h httpPort) AllProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.serv.AllProducts(r.Context())
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all products")
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	resp := map[string]any{
		"Products": toAllProductsResponse(products),
	}

	tmpl, err := template.New("name").Parse(allproductsTmpl)
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all products")
		log.Printf("cannot get list of all products: %s", err)
		return
	}

	tmpl.Execute(w, resp)
}

//go:embed tmpl/showProduct.gohtml
var showProductTmpl string

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

	resp := map[string]any{
		"Product": productsToResponse(product),
	}

	tmpl, err := template.New("name").Parse(showProductTmpl)
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get product")
		return
	}

	tmpl.Execute(w, resp)
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
