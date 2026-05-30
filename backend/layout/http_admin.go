package layout

import (
	"net/http"
	"sort"

	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
)

// requireAdmin is the access gate for every admin page. As of the
// customer/admin split it resolves the admin session cookie
// ("ecommerce_admin"), NOT the customer one — an admin and a customer
// session can coexist in the same browser and the two are wholly
// independent.
//
// On failure it has already written the response (a redirect to admin
// login for anonymous admins, or a redirect to /admin/change-password
// when the forced-reset gate is set) and returns ok=false; callers
// should simply return.
func (handler httpHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := handler.currentAdminEmail(r)
	if email == "" {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return "", false
	}

	// Force the password change before any admin work. A lookup
	// error here is treated as "not flagged" so a transient DB
	// hiccup doesn't lock the admin out of the panel.
	if must, mcpErr := handler.adminAuthSrv.MustChangePassword(r.Context(), email); mcpErr == nil && must {
		http.Redirect(w, r, "/admin/change-password", http.StatusSeeOther)
		return "", false
	}

	return email, true
}

// isAdmin is a defensive helper for templates: it reports whether the
// current request carries a valid admin session, swallowing any lookup
// error as a non-admin. Used by renderTemplate to expose `.IsAdmin`.
func (handler httpHandler) isAdmin(r *http.Request) bool {
	return handler.currentAdminID(r) != ""
}

// AdminDashboard renders the admin landing page with at-a-glance counts and
// the shared admin sub-navigation. It is the shell for the CRUD sections that
// later phases hang off /admin/*.
func (handler httpHandler) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	email, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	products, err := handler.catalogSrv.AllProducts(r.Context())
	if err != nil {
		products = nil
	}
	categories, err := handler.catalogSrv.Categories(r.Context())
	if err != nil {
		categories = nil
	}
	orders, err := handler.checkoutQry.ListAll(r.Context())
	if err != nil {
		orders = nil
	}
	// TodaysSales reads the analytics_daily_sales projection — the second
	// reader on the checkout event stream. A lookup error is swallowed
	// (the card is best-effort) and the dashboard simply omits the card.
	sales, err := handler.checkoutQry.TodaysSales(r.Context())
	if err != nil {
		sales = nil
	}
	todays := make([]checkoutQuery.DailySalesRow, 0, len(sales))
	for _, row := range sales {
		todays = append(todays, row)
	}
	sort.Slice(todays, func(i, j int) bool { return todays[i].Currency < todays[j].Currency })

	handler.renderAdminTemplate(w, r, "admin/dashboard", map[string]any{
		"Active":        "dashboard",
		"Email":         email,
		"ProductCount":  len(products),
		"CategoryCount": len(categories),
		"OrderCount":    len(orders),
		"TodaysSales":   todays,
	})
}
