package port

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

type AddToCartRequest struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"quantity"`
}

// @Router       /cart/{cartId} [post]
// @Accept       json
// @Produce      json
// @Param cart  body AddToCartRequest true "Cart"
func (h HTTP) AddToCart(w http.ResponseWriter, r *http.Request) {
	cartID := mux.Vars(r)["cartID"]

	req := AddToCartRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		https.BadRequest(w, err.Error())
		return
	}

	err := h.cart.AddToCart(r.Context(), cartID, req.ProductID, req.Qty)

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
