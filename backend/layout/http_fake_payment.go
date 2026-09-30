package layout

import (
	"errors"
	"net/http"
	"strings"

	checkoutDomain "github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	paymentsApp "github.com/bkielbasa/go-ecommerce/backend/payments/app"
	paymentsDomain "github.com/bkielbasa/go-ecommerce/backend/payments/domain"
	"github.com/gorilla/mux"
)

// FakePaymentSimulator renders the fake payment simulator page for a pending charge.
func (handler httpHandler) FakePaymentSimulator(w http.ResponseWriter, r *http.Request) {
	if handler.paymentsSrv == nil {
		https.InternalError(w, "internal-error", "payments service unavailable")
		return
	}
	chargeID := mux.Vars(r)["chargeID"]
	if chargeID == "" {
		http.NotFound(w, r)
		return
	}

	charge, err := handler.paymentsSrv.FindCharge(r.Context(), chargeID)
	if err != nil {
		if errors.Is(err, paymentsApp.ErrChargeNotFound) {
			http.NotFound(w, r)
			return
		}
		handler.logger.WithError(err).Error("cannot find fake charge")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	// If charge is already Succeeded or Failed: redirect to /order/{orderID}
	if charge.Status() == paymentsDomain.StatusSucceeded || charge.Status() == paymentsDomain.StatusFailed {
		http.Redirect(w, r, "/order/"+charge.OrderID(), http.StatusSeeOther)
		return
	}

	order, err := handler.checkoutQry.Find(r.Context(), charge.OrderID())
	if err != nil {
		if errors.Is(err, checkoutDomain.ErrOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		handler.logger.WithError(err).Error("cannot find order for fake charge")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	handler.renderTemplate(w, r, "payments/fake_simulator", map[string]any{
		"Charge": charge,
		"Order":  order,
	})
}

// FakePaymentConfirm simulates confirming a fake payment charge.
func (handler httpHandler) FakePaymentConfirm(w http.ResponseWriter, r *http.Request) {
	if handler.paymentsSrv == nil {
		https.InternalError(w, "internal-error", "payments service unavailable")
		return
	}
	chargeID := mux.Vars(r)["chargeID"]
	if chargeID == "" {
		http.NotFound(w, r)
		return
	}

	charge, err := handler.paymentsSrv.ConfirmCharge(r.Context(), chargeID)
	if err != nil {
		handler.logger.WithError(err).Error("cannot confirm fake charge")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	if err := handler.checkoutSrv.MarkPaid(r.Context(), charge.OrderID()); err != nil {
		handler.logger.WithError(err).Error("cannot mark order paid")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	handler.flash(w, r, "Payment confirmed successfully.", "info")
	http.Redirect(w, r, "/order/"+charge.OrderID(), http.StatusSeeOther)
}

// FakePaymentReject simulates declining a fake payment charge with a reason.
func (handler httpHandler) FakePaymentReject(w http.ResponseWriter, r *http.Request) {
	if handler.paymentsSrv == nil {
		https.InternalError(w, "internal-error", "payments service unavailable")
		return
	}
	chargeID := mux.Vars(r)["chargeID"]
	if chargeID == "" {
		http.NotFound(w, r)
		return
	}

	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		reason = "insufficient_funds"
	}

	charge, err := handler.paymentsSrv.RejectCharge(r.Context(), chargeID, reason)
	if err != nil {
		handler.logger.WithError(err).Error("cannot reject fake charge")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	if err := handler.checkoutSrv.MarkPaymentFailed(r.Context(), charge.OrderID(), reason); err != nil {
		handler.logger.WithError(err).Error("cannot mark order payment failed")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	handler.flash(w, r, "Payment declined: "+reason, "error")
	http.Redirect(w, r, "/order/"+charge.OrderID(), http.StatusSeeOther)
}
