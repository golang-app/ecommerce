package port

import (
	"github.com/bkielbasa/go-ecommerce/backend/cart/app"
)

type HTTP struct {
	cart app.CartService
}

func NewHTTP(storage app.CartStorage, pc app.ProductCatalog) HTTP {
	return HTTP{
		cart: app.NewCartService(storage, pc),
	}
}
