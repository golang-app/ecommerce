package layout

import (
	"errors"
	"net/http"
	"strings"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	fulfillmentApp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentDomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	promoapp "github.com/bkielbasa/go-ecommerce/backend/promo/app"
	promodomain "github.com/bkielbasa/go-ecommerce/backend/promo/domain"
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

	// Resolve the promo code (if any) BEFORE calling Place. We deliberately
	// re-read the cart here to derive the live subtotal — the checkout
	// service will read it again, but the promo Resolve call needs the
	// subtotal NOW to compute the discount amount for the eventual
	// OrderPlaced event. A rejected code never reaches checkoutSrv.Place;
	// the customer gets a flash message and the form re-renders.
	discount := promodomain.Discount{}
	promoCode := strings.TrimSpace(r.FormValue("promo_code"))
	if promoCode != "" {
		cart, cartErr := handler.cartSrv.Get(r.Context(), sessID)
		if cartErr != nil || cart == nil || len(cart.Items()) == 0 {
			http.Redirect(w, r, "/cart", http.StatusSeeOther)
			return
		}
		subtotal := cart.TotalPrice().Amount()
		d, perr := handler.promoSrv.Resolve(r.Context(), promoCode, customerID, subtotal, method.Cost())
		if perr != nil {
			handler.flash(w, r, promoCodeErrorMessage(perr), "error")
			http.Redirect(w, r, "/checkout", http.StatusSeeOther)
			return
		}
		discount = d
	}

	order, err := handler.checkoutSrv.Place(r.Context(), sessID, customerID, cardNumber, shipTo, method, payMethod, discount)
	if errors.Is(err, checkoutDomain.ErrCartEmpty) {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}
	// The HTTP form already validates address / payment-method shape via
	// domain.NewAddress / PaymentMethodByCode above, but the Aggregate
	// Factory re-checks them as a belt-and-braces guard so a malformed
	// caller (an alternate entry point, a future API) cannot smuggle an
	// invalid Order into the event log. Map the sentinels to user-facing
	// remediations rather than a generic 500.
	if errors.Is(err, checkoutDomain.ErrAddressRequired) {
		handler.flash(w, r, "please provide a shipping address for the chosen shipping method", "error")
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}
	if errors.Is(err, checkoutDomain.ErrPaymentMethodRequired) {
		handler.flash(w, r, "please choose a payment method", "error")
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}
	if errors.Is(err, checkoutDomain.ErrChannelRequired) {
		// The web flow always sets channel="web"; reaching this branch
		// would indicate a programmer error. Render a generic message
		// and send the customer back to the form rather than 500.
		handler.flash(w, r, "could not place the order; please try again", "error")
		http.Redirect(w, r, "/checkout", http.StatusSeeOther)
		return
	}
	if errors.Is(err, pcdomain.ErrInsufficientStock) {
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

	orders, err := handler.checkoutQry.ListByCustomer(r.Context(), customerID)
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

	order, err := handler.checkoutQry.Find(r.Context(), orderID)
	if errors.Is(err, checkoutDomain.ErrOrderNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	// The owner may cancel a paid order. Cancellation is only sensible
	// before the fulfillment Process Manager has shipped the parcel —
	// the admin's refund flow takes over after that.
	customerID := handler.currentCustomerID(r)
	canCancel := order.Status() == checkoutDomain.StatusPaid &&
		customerID != "" && order.CustomerID() == customerID

	// Fulfillment summary: shown alongside the commercial status when
	// the OnOrderPaid subscriber has spawned a record. Anonymous /
	// unpaid orders simply have no fulfillment to display.
	var (
		fulfillment    fulfillmentDomain.Fulfillment
		hasFulfillment bool
	)
	if handler.fulfillmentSrv != nil {
		ff, ferr := handler.fulfillmentSrv.ByOrder(r.Context(), orderID)
		switch {
		case errors.Is(ferr, fulfillmentApp.ErrNotFound):
			// not yet spawned
		case ferr != nil:
			handler.logger.WithError(ferr).Warn("cannot load fulfillment for order page")
		default:
			fulfillment = ff
			hasFulfillment = true
			// Once a shipment is in flight (labeled or later) the
			// customer can no longer cancel from this page.
			if fulfillment.Status() != fulfillmentDomain.StatusScheduled {
				canCancel = false
			}
		}
	}

	handler.renderTemplate(w, r, "order/show", map[string]any{
		"Order":          order,
		"Fulfillment":    fulfillment,
		"HasFulfillment": hasFulfillment,
		"CanCancel":      canCancel,
	})
}

// promoCodeErrorMessage turns a promo/app sentinel error into a
// customer-facing flash. Anything we don't recognise falls back to a
// neutral "invalid promo code" so internal storage errors do not leak.
func promoCodeErrorMessage(err error) string {
	switch {
	case errors.Is(err, promoapp.ErrCodeNotFound):
		return "That promo code isn't recognised."
	case errors.Is(err, promoapp.ErrCodeExpired):
		return "That promo code is not currently active."
	case errors.Is(err, promoapp.ErrCodeMaxUsesReached):
		return "That promo code has reached its redemption limit."
	case errors.Is(err, promoapp.ErrCodeCustomerLimit):
		return "You've already used that promo code."
	case errors.Is(err, promoapp.ErrCodeAnonymous):
		return "Please sign in before using a promo code."
	default:
		return "That promo code couldn't be applied."
	}
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
