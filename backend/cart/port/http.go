package port

import "github.com/bkielbasa/go-ecommerce/backend/cart/app"

type AddToCartRequest struct{}

type HTTP struct {
	serv app.CartService
}

func NewHTTP(storage app.CartStorage) HTTP {
	return HTTP{
		serv: app.NewCartService(storage, nil),
	}
}
