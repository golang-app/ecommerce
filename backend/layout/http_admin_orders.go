package layout

import (
	"errors"
	"net/http"
	"strings"

	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	fulfillmentApp "github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	fulfillmentDomain "github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
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
// fulfillment controls — buttons appropriate to the fulfillment's
// current operational status (ship for scheduled, deliver for shipped,
// refund for any active state). Commercial state (paid / cancelled)
// comes from the order; operational state comes from the fulfillment
// record (which is spawned by the OnOrderPaid subscriber).
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

	// Fulfillment is spawned by the OnOrderPaid subscriber; for orders
	// that never paid (pending / failed / cancelled) there is no row
	// yet, which is fine — the template renders the operational
	// section conditionally.
	var (
		fulfillment    fulfillmentDomain.Fulfillment
		hasFulfillment bool
	)
	if handler.fulfillmentSrv != nil {
		ff, ferr := handler.fulfillmentSrv.ByOrder(r.Context(), orderID)
		switch {
		case errors.Is(ferr, fulfillmentApp.ErrNotFound):
			// not paid yet / process manager hasn't reacted yet
		case ferr != nil:
			https.InternalError(w, "internal-error", ferr.Error())
			return
		default:
			fulfillment = ff
			hasFulfillment = true
		}
	}

	commercial := order.Status()
	canCancel := commercial == checkoutDomain.StatusPaid && !hasFulfillment
	// Once a fulfillment exists, ship/deliver/refund are gated on its
	// operational status rather than the order's commercial status.
	canShip := hasFulfillment && fulfillment.Status() == fulfillmentDomain.StatusScheduled
	canDeliver := hasFulfillment && fulfillment.Status() == fulfillmentDomain.StatusShipped
	canRefund := hasFulfillment && fulfillment.Status() != fulfillmentDomain.StatusRefunded &&
		fulfillment.Status() != fulfillmentDomain.StatusReturned
	handler.renderAdminTemplate(w, r, "admin/order_detail", map[string]any{
		"Active":         "orders",
		"Email":          email,
		"Order":          order,
		"Fulfillment":    fulfillment,
		"HasFulfillment": hasFulfillment,
		"CanCancel":      canCancel,
		"CanShip":        canShip,
		"CanDeliver":     canDeliver,
		"CanRefund":      canRefund,
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

// AdminShipOrder drives the fulfillment Process Manager: it records
// the carrier + tracking code on the scheduled fulfillment (Label)
// and then transitions it to shipped (Ship). Two calls back-to-back
// because the underlying state machine keeps Label and Ship distinct
// — a future "labeled but not yet handed to carrier" pause can land
// without changing this handler shape.
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
	if err := handler.fulfillmentSrv.Label(r.Context(), orderID, carrier, tracking); err != nil {
		switch {
		case errors.Is(err, fulfillmentApp.ErrNotFound):
			handler.flash(w, r, "Fulfillment not found for this order.", "error")
			http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
			return
		case errors.Is(err, fulfillmentDomain.ErrInvalidTransition):
			handler.flash(w, r, "This order cannot be shipped from its current status.", "error")
			http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
			return
		default:
			https.InternalError(w, "internal-error", err.Error())
			return
		}
	}
	if err := handler.fulfillmentSrv.Ship(r.Context(), orderID); err != nil {
		switch {
		case errors.Is(err, fulfillmentDomain.ErrInvalidTransition):
			handler.flash(w, r, "This order cannot be shipped from its current status.", "error")
		default:
			https.InternalError(w, "internal-error", err.Error())
			return
		}
	} else {
		handler.flash(w, r, "Order marked as shipped.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}

// AdminDeliverOrder transitions a shipped fulfillment to delivered.
func (handler httpHandler) AdminDeliverOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}
	orderID := mux.Vars(r)["orderID"]
	err := handler.fulfillmentSrv.Deliver(r.Context(), orderID)
	switch {
	case errors.Is(err, fulfillmentApp.ErrNotFound):
		handler.flash(w, r, "Fulfillment not found for this order.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, fulfillmentDomain.ErrInvalidTransition):
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

// AdminRefundOrder refunds the fulfillment for an order; the
// fulfillment service also releases the reserved stock back to the
// catalogue via the wired StockReleaser port.
//
// Idempotency. This endpoint participates in the HTTP-boundary
// Idempotency-Key contract (see internal/idempotency): a client may
// send the same `Idempotency-Key` header on a retry and the
// originally-recorded response will be replayed instead of issuing a
// second refund attempt. Refunds are the highest-stakes admin POST in
// the system — a network blip after the refund commits is exactly the
// situation the contract exists to neutralise.
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
	err := handler.fulfillmentSrv.Refund(r.Context(), orderID, reason)
	switch {
	case errors.Is(err, fulfillmentApp.ErrNotFound):
		handler.flash(w, r, "Fulfillment not found for this order.", "error")
		http.Redirect(w, r, "/admin/orders", http.StatusSeeOther)
		return
	case errors.Is(err, fulfillmentDomain.ErrInvalidTransition):
		handler.flash(w, r, "This order cannot be refunded from its current status.", "error")
	case err != nil:
		https.InternalError(w, "internal-error", err.Error())
		return
	default:
		handler.flash(w, r, "Order refunded.", "info")
	}
	http.Redirect(w, r, "/admin/orders/"+orderID, http.StatusSeeOther)
}
