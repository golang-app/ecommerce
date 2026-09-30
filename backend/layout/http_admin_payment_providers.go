package layout

import (
	"net/http"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	paymentsDomain "github.com/bkielbasa/go-ecommerce/backend/payments/domain"
	"github.com/gorilla/mux"
)

// AdminPaymentProviders renders the payment providers administration page.
func (handler httpHandler) AdminPaymentProviders(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	var providers []paymentsDomain.ProviderConfig
	if handler.paymentsSrv != nil {
		p, err := handler.paymentsSrv.ListProviders(r.Context())
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
		} else {
			providers = p
		}
	}

	handler.renderAdminTemplate(w, r, "admin/payment_providers", map[string]any{
		"Active":    "payment-providers",
		"Email":     email,
		"Providers": providers,
	})
}

// AdminUpdatePaymentProvider updates the configuration and enabled status of a payment provider.
func (handler httpHandler) AdminUpdatePaymentProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	if handler.paymentsSrv == nil {
		handler.flash(w, r, "payments service not wired", "error")
		http.Redirect(w, r, "/admin/payment-providers", http.StatusSeeOther)
		return
	}

	id := mux.Vars(r)["id"]
	enabledVal := r.FormValue("enabled")
	enabled := enabledVal == "on" || enabledVal == "1" || enabledVal == "true"

	config := make(map[string]string)
	switch id {
	case paymentsDomain.ProviderStripe:
		config["fail_card_ending_in"] = strings.TrimSpace(r.FormValue("fail_card_ending_in"))
		config["publishable_key"] = strings.TrimSpace(r.FormValue("publishable_key"))
		config["secret_key"] = strings.TrimSpace(r.FormValue("secret_key"))
		config["webhook_secret"] = strings.TrimSpace(r.FormValue("webhook_secret"))
	case paymentsDomain.ProviderFake:
		config["name"] = strings.TrimSpace(r.FormValue("name"))
		config["description"] = strings.TrimSpace(r.FormValue("description"))
	default:
		for k, v := range r.PostForm {
			if k != "csrf_token" && k != "enabled" && len(v) > 0 {
				config[k] = strings.TrimSpace(v[0])
			}
		}
	}

	if err := handler.paymentsSrv.UpdateProvider(r.Context(), id, enabled, config); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Payment provider updated successfully.", "success")
	}

	http.Redirect(w, r, "/admin/payment-providers", http.StatusSeeOther)
}
