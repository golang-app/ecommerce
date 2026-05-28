package layout

import (
	"errors"
	"net/http"
	"strings"

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
	handler.renderAdminTemplate(w, r, "admin/orders", map[string]any{
		"Active": "orders",
		"Email":  email,
		"Orders": orders,
	})
}

// AdminOrderDetail renders a single order's detail with the admin
// fulfillment controls — buttons appropriate to the order's current status
// (cancel/ship for paid, mark-delivered for shipped, refund for any paid
// or fulfilled order).
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
	status := order.Status()
	canRefund := status == checkoutDomain.StatusPaid ||
		status == checkoutDomain.StatusShipped ||
		status == checkoutDomain.StatusDelivered
	handler.renderAdminTemplate(w, r, "admin/order_detail", map[string]any{
		"Active":       "orders",
		"Email":        email,
		"Order":        order,
		"CanCancel":    status == checkoutDomain.StatusPaid,
		"CanShip":      status == checkoutDomain.StatusPaid,
		"CanDeliver":   status == checkoutDomain.StatusShipped,
		"CanRefund":    canRefund,
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

// AdminShipOrder marks a paid order as shipped, optionally recording the
// carrier and tracking code submitted on the form.
func (handler httpHandler) AdminShipOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	orderID := mux.Vars(r)["orderID"]
	carrier := r.FormValue("carrier")
	tracking := r.FormValue("tracking_code")
	err := handler.checkoutSrv.MarkShipped(r.Context(), orderID, carrier, tracking)
	switch {
	case errors.Is(err, checkoutDomain.ErrOrderNotFound):
		handler.flash(w, r, "Order not found.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, checkoutDomain.ErrOrderNotShippable):
		handler.flash(w, r, "This order cannot be shipped from its current status.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Order marked as shipped.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}

// AdminDeliverOrder marks a shipped order as delivered.
func (handler httpHandler) AdminDeliverOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	orderID := mux.Vars(r)["orderID"]
	err := handler.checkoutSrv.MarkDelivered(r.Context(), orderID)
	switch {
	case errors.Is(err, checkoutDomain.ErrOrderNotFound):
		handler.flash(w, r, "Order not found.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, checkoutDomain.ErrOrderNotDeliverable):
		handler.flash(w, r, "This order cannot be marked delivered from its current status.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Order marked as delivered.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}

// AdminInventory renders the inventory audit-trail page: the 200 most recent
// stock movements. Optional ?variant=<id> query filters to a single variant.
func (handler httpHandler) AdminInventory(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}
	variantID := strings.TrimSpace(r.URL.Query().Get("variant"))
	movements, err := handler.catalogSrv.ListStockMovements(r.Context(), variantID, 200)
	if err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	handler.renderAdminTemplate(w, r, "admin/stock_movements", map[string]any{
		"Active":    "inventory",
		"Email":     email,
		"Movements": movements,
		"VariantID": variantID,
	})
}

// AdminRefundOrder refunds a paid/shipped/delivered order; the underlying
// service also releases the reserved stock back to the catalogue.
func (handler httpHandler) AdminRefundOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}
	orderID := mux.Vars(r)["orderID"]
	reason := r.FormValue("reason")
	err := handler.checkoutSrv.Refund(r.Context(), orderID, reason)
	switch {
	case errors.Is(err, checkoutDomain.ErrOrderNotFound):
		handler.flash(w, r, "Order not found.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, checkoutDomain.ErrOrderNotRefundable):
		handler.flash(w, r, "This order cannot be refunded from its current status.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Order refunded.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}
