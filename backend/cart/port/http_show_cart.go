package port

import (
	_ "embed"
	"html/template"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type showCartResponse struct {
	Items []showCartItemResponse `json:"items"`
}

type showCartItemResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Price    price  `json:"price"`
	Quantity int    `json:"quantity"`
}

type price struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

//go:embed tmpl/cartBudge.gohtml
var cartCouterTmpl string

func (h HTTP) Budge(w http.ResponseWriter, r *http.Request) {
	cartID := cartIDFromCookies(w, r)

	items, err := h.cart.Items(r.Context(), cartID)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	counter := 0

	for _, item := range items {
		counter += item.Quantity()
	}

	if counter == 0 {
		return
	}

	resp := map[string]any{
		"Product": counter,
	}

	tmpl, err := template.New("name").Parse(cartCouterTmpl)
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get product")
		return
	}

	tmpl.Execute(w, resp)
}

func (h HTTP) ShowCart(w http.ResponseWriter, r *http.Request) {
	cartID := cartIDFromCookies(w, r)

	items, err := h.cart.Items(r.Context(), cartID)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	respItems := make([]showCartItemResponse, 0, len(items))

	for _, item := range items {
		respItems = append(respItems, showCartItemResponse{
			ID:   item.Product().ID(),
			Name: item.Product().Name(),
			Price: price{
				Amount:   item.Product().Price().Amount(),
				Currency: item.Product().Price().Currency(),
			},
			Quantity: item.Quantity(),
		})
	}

	https.OK(w, showCartResponse{Items: respItems})
}
