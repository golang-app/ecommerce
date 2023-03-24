package port

import (
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
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

func (h HTTP) ShowCart(w http.ResponseWriter, r *http.Request) {
	cartID := mux.Vars(r)["cartID"]

	items, err := h.cart.Items(r.Context(), cartID)
	if err != nil {
		https.InternalError(w, err.Error())
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
