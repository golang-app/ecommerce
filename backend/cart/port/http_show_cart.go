package port

import (
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type showCartResponse struct {
	Items []showCartItemResponse `json:"items"`
}

type showCartItemResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
}

func (h HTTP) ShowCart(w http.ResponseWriter, r *http.Request) {
	cartID := cartID(r)

	items, err := h.cart.Items(r.Context(), cartID)
	if err != nil {
		https.InternalError(w, err.Error())
		return
	}

	respItems := make([]showCartItemResponse, 0, len(items))

	for _, item := range items {
		respItems = append(respItems, showCartItemResponse{
			ID:       item.Product().ID(),
			Name:     item.Product().Name(),
			Price:    item.Product().Price().Amount(),
			Currency: item.Product().Price().Currency(),
		})
	}

	https.OK(w, showCartResponse{Items: respItems})
}
