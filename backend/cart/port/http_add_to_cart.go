package port

import (
	"errors"
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

func (h HTTP) AddToCart(w http.ResponseWriter, r *http.Request) {
	cartID := cartIDFromCookies(w, r)
	productID := mux.Vars(r)["productID"]

	err := h.cart.AddToCart(r.Context(), cartID, productID, 1)

	if errors.Is(err, domain.ErrProductNotFound) {
		https.NotFound(w, "cart-not-found", err.Error())
		return
	}

	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		log.Print(err)
		return
	}

	w.Header().Add("HX-Trigger", "cartBudge")

	https.NoContent(w)
}
