package port

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type AddToCartRequest struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"qty"`
}

func (h HTTP) AddToCart(w http.ResponseWriter, r *http.Request) {
	sessID := cartID(r)

	req := AddToCartRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		https.BadRequest(w, err.Error())
		return
	}

	err := h.cart.AddToCart(r.Context(), sessID, req.ProductID, req.Qty)

	if errors.Is(err, domain.ErrProductNotFound) {
		https.NotFound(w, err.Error())
		return
	}

	if err != nil {
		https.InternalError(w, err.Error())
		log.Print(err)
		return
	}

	https.NoContent(w)
}
