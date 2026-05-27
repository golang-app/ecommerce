package layout

import (
	"errors"
	"net/http"
	"strings"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
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

	data := map[string]any{
		"Cart":            cart,
		"ShippingMethods": checkoutDomain.ShippingMethods(),
		"PaymentMethods":  checkoutDomain.PaymentMethods(),
	}

	// Prefill the shipping form from the logged-in customer's default saved
	// address, if they have one.
	if customerID := handler.currentCustomerID(r); customerID != "" {
		if addr, ok, err := handler.shipSrv.Default(r.Context(), customerID); err == nil && ok {
			data["ShipTo"] = addr
		}
	}

	handler.renderTemplate(w, r, "checkout/show", data)
}

func (handler httpHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	sessID := cartIDFromCookies(w, r)
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	cardNumber := r.FormValue("card_number")
	customerID := handler.currentCustomerID(r) // empty for anonymous

	method, err := checkoutDomain.ShippingMethodByCode(r.FormValue("ship_method"))
	if err != nil {
		session, _ := store.Get(r, "ecommerce")
		session.AddFlash("please choose a shipping method", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}

	// Pickup needs no address; for delivery methods the address is required.
	var shipTo checkoutDomain.Address
	if method.RequiresAddress() {
		shipTo, err = checkoutDomain.NewAddress(
			r.FormValue("ship_name"),
			r.FormValue("ship_street1"),
			r.FormValue("ship_street2"),
			r.FormValue("ship_city"),
			r.FormValue("ship_zip"),
			r.FormValue("ship_country"),
		)
		if err != nil {
			session, _ := store.Get(r, "ecommerce")
			session.AddFlash(err.Error(), "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/checkout", http.StatusSeeOther)
			return
		}
	}

	payMethod, err := checkoutDomain.PaymentMethodByCode(r.FormValue("payment_method"))
	if err != nil {
		session, _ := store.Get(r, "ecommerce")
		session.AddFlash("please choose a payment method", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}

	// Card details are only required for the card payment method.
	if payMethod.RequiresCard() && strings.TrimSpace(cardNumber) == "" {
		session, _ := store.Get(r, "ecommerce")
		session.AddFlash("card number is required for card payments", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}

	order, err := handler.checkoutSrv.Place(r.Context(), sessID, customerID, cardNumber, shipTo, method, payMethod)
	if errors.Is(err, checkoutDomain.ErrCartEmpty) {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	if errors.Is(err, productcatalog.ErrInsufficientStock) {
		handler.flash(w, r, "Sorry — an item in your cart just went out of stock. Please review your cart.", "error")
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

	// The owner may cancel a paid order.
	customerID := handler.currentCustomerID(r)
	canCancel := order.Status() == checkoutDomain.StatusPaid &&
		customerID != "" && order.CustomerID() == customerID

	handler.renderTemplate(w, r, "order/show", map[string]any{
		"Order":     order,
		"CanCancel": canCancel,
	})
}

func (handler httpHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	customerID, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	orderID := mux.Vars(r)["orderID"]

	err := handler.checkoutSrv.Cancel(r.Context(), orderID, customerID)
	switch {
	case errors.Is(err, checkoutDomain.ErrOrderNotFound):
		http.NotFound(w, r)
		return
	case errors.Is(err, checkoutDomain.ErrOrderNotCancellable):
		handler.flash(w, r, "This order can no longer be cancelled.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Your order has been cancelled.", "info")
	}
	http.Redirect(w, r, "/order/"+orderID, http.StatusSeeOther)
}
