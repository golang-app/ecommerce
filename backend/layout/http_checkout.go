package layout

import (
	"errors"
	"net/http"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

// currentCustomerID returns the authenticated customer's id from the
// session cookie, or "" if no valid session is present.
func (handler httpHandler) currentCustomerID(r *http.Request) string {
	c, err := store.Get(r, "ecommerce")
	if err != nil {
		return ""
	}
	sessID, _ := c.Values["session_id"].(string)
	if sessID == "" {
		return ""
	}
	sess, err := handler.authSrv.FindByToken(r.Context(), sessID)
	if err != nil || sess == nil || sess.Expired() {
		return ""
	}
	return sess.CustomerID()
}

func (handler httpHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	sessID := cartIDFromCookies(w, r)
	cart, err := handler.cartSrv.Get(r.Context(), sessID)
	if errors.Is(err, cartDomain.ErrCartNotFound) {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	if len(cart.Items()) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	handler.renderTemplate(w, r, "checkout/show", map[string]any{
		"Cart": cart,
	})
}

func (handler httpHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	sessID := cartIDFromCookies(w, r)
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	cardNumber := r.FormValue("card_number")
	customerID := handler.currentCustomerID(r) // empty for anonymous

	order, err := handler.checkoutSrv.Place(r.Context(), sessID, customerID, cardNumber)
	if errors.Is(err, checkoutDomain.ErrCartEmpty) {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	// Refresh the cart-count badge in the header.
	w.Header().Add("HX-Trigger", "cartBudge")
	http.Redirect(w, r, "/order/"+order.ID(), http.StatusSeeOther)
}

func (handler httpHandler) Orders(w http.ResponseWriter, r *http.Request) {
	customerID := handler.currentCustomerID(r)
	if customerID == "" {
		// Stash the intent so we could return them here post-login later.
		// For now just bounce to the login page.
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	orders, err := handler.checkoutSrv.ListByCustomer(r.Context(), customerID)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	handler.renderTemplate(w, r, "order/index", map[string]any{
		"Orders": orders,
	})
}

func (handler httpHandler) Order(w http.ResponseWriter, r *http.Request) {
	orderID := mux.Vars(r)["orderID"]

	order, err := handler.checkoutSrv.Find(r.Context(), orderID)
	if errors.Is(err, checkoutDomain.ErrOrderNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	handler.renderTemplate(w, r, "order/show", map[string]any{
		"Order": order,
	})
}
