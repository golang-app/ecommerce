package layout

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

type ShippingProviderView struct {
	Code            string
	Label           string
	Cost            int64
	CostFormatted   string
	Carrier         string
	RequiresAddress bool
	IsEnabled       bool
}

// AdminShippingProviders renders the shipping providers administration page.
func (handler httpHandler) AdminShippingProviders(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	var views []ShippingProviderView
	if handler.checkoutSrv != nil {
		methods, err := handler.checkoutSrv.ListShippingMethods(r.Context())
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
		} else {
			views = make([]ShippingProviderView, 0, len(methods))
			for _, m := range methods {
				views = append(views, ShippingProviderView{
					Code:            m.Code(),
					Label:           m.Label(),
					Cost:            m.Cost(),
					CostFormatted:   fmt.Sprintf("%.2f", float64(m.Cost())/100.0),
					Carrier:         m.Carrier(),
					RequiresAddress: m.RequiresAddress(),
					IsEnabled:       m.IsEnabled(),
				})
			}
		}
	}

	handler.renderAdminTemplate(w, r, "admin/shipping_providers", map[string]any{
		"Active":    "shipping-providers",
		"Email":     email,
		"Providers": views,
	})
}

// AdminUpdateShippingProvider updates the shipping method configuration.
func (handler httpHandler) AdminUpdateShippingProvider(w http.ResponseWriter, r *http.Request) {
	if _, ok := handler.requireAdmin(w, r); !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	if handler.checkoutSrv == nil {
		handler.flash(w, r, "checkout service not wired", "error")
		http.Redirect(w, r, "/admin/shipping-providers", http.StatusSeeOther)
		return
	}

	code := mux.Vars(r)["code"]
	enabledVal := r.FormValue("enabled")
	enabled := enabledVal == "1" || enabledVal == "on" || enabledVal == "true"
	label := strings.TrimSpace(r.FormValue("label"))
	carrier := strings.TrimSpace(r.FormValue("carrier"))

	if label == "" {
		handler.flash(w, r, "Provider label cannot be empty.", "error")
		http.Redirect(w, r, "/admin/shipping-providers", http.StatusSeeOther)
		return
	}

	if !enabled {
		methods, err := handler.checkoutSrv.ListShippingMethods(r.Context())
		if err == nil {
			enabledCount := 0
			var isCurrentEnabled bool
			for _, m := range methods {
				if m.IsEnabled() {
					enabledCount++
				}
				if m.Code() == code && m.IsEnabled() {
					isCurrentEnabled = true
				}
			}
			if isCurrentEnabled && enabledCount <= 1 {
				handler.flash(w, r, "Cannot disable the last active shipping provider.", "error")
				http.Redirect(w, r, "/admin/shipping-providers", http.StatusSeeOther)
				return
			}
		}
	}

	var costMinor int64
	costStr := strings.TrimSpace(r.FormValue("cost"))
	if code == "pickup" && costStr == "" {
		costMinor = 0
	} else {
		var err error
		costMinor, err = parsePriceMinorUnits(costStr)
		if err != nil {
			handler.flash(w, r, err.Error(), "error")
			http.Redirect(w, r, "/admin/shipping-providers", http.StatusSeeOther)
			return
		}
	}

	if err := handler.checkoutSrv.UpdateShippingMethod(r.Context(), code, enabled, label, costMinor, carrier); err != nil {
		handler.flash(w, r, err.Error(), "error")
	} else {
		handler.flash(w, r, "Shipping provider updated successfully.", "success")
	}

	http.Redirect(w, r, "/admin/shipping-providers", http.StatusSeeOther)
}
