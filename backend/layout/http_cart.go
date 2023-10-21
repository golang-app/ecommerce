package layout

import (
	"errors"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

func (handler httpHandler) AddToCart(w http.ResponseWriter, r *http.Request) {
	cartID := cartIDFromCookies(w, r)
	productID := mux.Vars(r)["cartID"]

	err := handler.cartSrv.AddToCart(r.Context(), cartID, productID, 1)

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

func (handler httpHandler) Budge(w http.ResponseWriter, r *http.Request) {
	cartID := cartIDFromCookies(w, r)

	items, err := handler.cartSrv.Items(r.Context(), cartID)
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
		"Counter": counter,
	}

	err = tmpl.ExecuteTemplate(w, "budge.gohtml", resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
