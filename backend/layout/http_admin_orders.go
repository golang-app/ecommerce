package layout

import (
	"errors"
	"net/http"

	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

// AdminOrders renders the admin order list: every order newest-first.
func (handler httpHandler) AdminOrders(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	orders, err := handler.checkoutQry.ListAll(r.Context())
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderTemplate(w, r, "admin/orders", map[string]any{
		"Active": "orders",
		"Email":  email,
		"Orders": orders,
	})
}

// AdminOrderDetail renders a single order's detail with an admin cancel form
// for paid orders.
func (handler httpHandler) AdminOrderDetail(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
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
	handler.renderTemplate(w, r, "admin/order_detail", map[string]any{
		"Active":    "orders",
		"Email":     email,
		"Order":     order,
		"CanCancel": order.Status() == checkoutDomain.StatusPaid,
	})
}

// AdminCancelOrder cancels any order as admin and redirects back to its detail.
func (handler httpHandler) AdminCancelOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	orderID := mux.Vars(r)["orderID"]
	err := handler.checkoutSrv.AdminCancel(r.Context(), orderID)
	switch {
	case errors.Is(err, checkoutDomain.ErrOrderNotFound):
		handler.flash(w, r, "Order not found.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, checkoutDomain.ErrOrderNotCancellable):
		handler.flash(w, r, "This order can no longer be cancelled.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Order cancelled.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}
