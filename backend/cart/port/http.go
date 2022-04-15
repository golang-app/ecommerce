package port

import (
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

type AddToCartRequest struct{}

type HTTP struct {
	storage app.CartStorage
	pc      app.ProductCatalog
}

func NewHTTP(storage app.CartStorage, pc app.ProductCatalog) HTTP {
	return HTTP{
		storage: storage,
		pc:      pc,
	}
}

func (h HTTP) ShowCart(w http.ResponseWriter, r *http.Request) {
	sessID := sessionID(r)
	cart, err := h.storage.Get(r.Context(), sessID)
	if err != nil {
		https.InternalError(w, "cannot read the cart")
		return
	}
	serv := app.NewCartService(cart, h.pc)

	price, err := serv.TotalPrice()
	if err != nil {
		https.InternalError(w, "cannot calculate total price")
		return
	}

	https.OK(w, price)
}
