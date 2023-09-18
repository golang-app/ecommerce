package port

import (
	"math/rand"
	"net/http"
	"time"

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

func cartIDFromCookies(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("cart_id")
	if err == nil {
		return cookie.Value
	}

	cartID := "cart-" + randomString(16)

	http.SetCookie(w, &http.Cookie{
		Name:  "cart_id",
		Value: cartID,
	})

	return cartID
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
