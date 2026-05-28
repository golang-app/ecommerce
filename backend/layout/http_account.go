package layout

import (
	"net/http"

	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

// requireLogin resolves the current customer or redirects to the login page.
// If the customer still has must_change_password set, the gate diverts them
// to /auth/change-password so the forced reset cannot be skipped by visiting
// /account/* directly. The change-password handlers themselves use
// requireLoginAllowMustChange to avoid an infinite redirect loop.
func (handler httpHandler) requireLogin(w http.ResponseWriter, r *http.Request) (string, bool) {
	cid := handler.currentCustomerID(r)
	if cid == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return "", false
	}
	if must, err := handler.authSrv.MustChangePassword(r.Context(), cid); err == nil && must {
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return "", false
	}
	return cid, true
}

// requireLoginAllowMustChange is the variant used by the forced-reset page
// itself: it confirms the caller is logged in but does NOT redirect when the
// must_change_password flag is set (which is exactly the state in which the
// caller is allowed to be on this page).
func (handler httpHandler) requireLoginAllowMustChange(w http.ResponseWriter, r *http.Request) (string, bool) {
	cid := handler.currentCustomerID(r)
	if cid == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return "", false
	}
	return cid, true
}

func (handler httpHandler) flash(w http.ResponseWriter, r *http.Request, msg, kind string) {
	session, _ := store.Get(r, "ecommerce")
	if kind == "error" {
		session.AddFlash(msg, "error")
	} else {
		session.AddFlash(msg)
	}
	_ = session.Save(r, w)
}

func (handler httpHandler) AccountOverview(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}

	var latest checkoutQuery.OrderSummary
	if orders, err := handler.checkoutQry.ListByCustomer(r.Context(), cid); err == nil && len(orders) > 0 {
		latest = orders[0]
	}
	def, _, _ := handler.shipSrv.Default(r.Context(), cid)

	handler.renderTemplate(w, r, "account/overview", map[string]any{
		"Active":         "overview",
		"Email":          cid,
		"LatestOrder":    latest,
		"DefaultAddress": def,
	})
}

func (handler httpHandler) AccountOrders(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	orders, err := handler.checkoutQry.ListByCustomer(r.Context(), cid)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderTemplate(w, r, "account/orders", map[string]any{
		"Active": "orders",
		"Email":  cid,
		"Orders": orders,
	})
}

func (handler httpHandler) AccountAddresses(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	addrs, err := handler.shipSrv.List(r.Context(), cid)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderTemplate(w, r, "account/addresses", map[string]any{
		"Active":    "addresses",
		"Email":     cid,
		"Addresses": addrs,
	})
}

func (handler httpHandler) AccountAddAddress(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := handler.shipSrv.Add(r.Context(), cid,
		r.FormValue("name"), r.FormValue("street1"), r.FormValue("street2"),
		r.FormValue("city"), r.FormValue("zip"), r.FormValue("country"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Address saved", "info")
	}
	http.Redirect(w, r, "/account/addresses", http.StatusSeeOther)
}

func (handler httpHandler) AccountEditAddressForm(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	addr, err := handler.shipSrv.Get(r.Context(), cid, id)
	if err != nil {
		http.Redirect(w, r, "/account/addresses", http.StatusSeeOther)
		return
	}
	handler.renderTemplate(w, r, "account/address_edit", map[string]any{
		"Active":  "addresses",
		"Email":   cid,
		"Address": addr,
	})
}

func (handler httpHandler) AccountUpdateAddress(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	_ = r.ParseForm()
	err := handler.shipSrv.Edit(r.Context(), cid, id,
		r.FormValue("name"), r.FormValue("street1"), r.FormValue("street2"),
		r.FormValue("city"), r.FormValue("zip"), r.FormValue("country"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
		http.Redirect(w, r, "/account/addresses/"+id+"/edit", http.StatusSeeOther)
		return
	}
	handler.flash(w, r, "Address updated", "info")
	http.Redirect(w, r, "/account/addresses", http.StatusSeeOther)
}

func (handler httpHandler) AccountDeleteAddress(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.shipSrv.Remove(r.Context(), cid, id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Address removed", "info")
	}
	http.Redirect(w, r, "/account/addresses", http.StatusSeeOther)
}

func (handler httpHandler) AccountSetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	id := mux.Vars(r)["id"]
	if err := handler.shipSrv.SetDefault(r.Context(), cid, id); err != nil {
		handler.flash(w, r, err.Error(), "error")
	}
	http.Redirect(w, r, "/account/addresses", http.StatusSeeOther)
}

func (handler httpHandler) AccountDetails(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	handler.renderTemplate(w, r, "account/details", map[string]any{
		"Active": "details",
		"Email":  cid,
	})
}

func (handler httpHandler) AccountChangePassword(w http.ResponseWriter, r *http.Request) {
	cid, ok := handler.requireLogin(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()
	err := handler.authSrv.ChangePassword(r.Context(), cid, r.FormValue("current_password"), r.FormValue("new_password"))
	if err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Password updated", "info")
	}
	http.Redirect(w, r, "/account/details", http.StatusSeeOther)
}
