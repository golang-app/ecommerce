package layout

import (
	"net/http"
	"sort"

	checkoutQuery "github.com/bkielbasa/go-ecommerce/backend/checkout/query"
)

// requireAdmin is the access gate for every admin page. It resolves the
// current customer and confirms they hold the admin flag. On failure it has
// already written the response (a redirect to login for anonymous users, or a
// 403 for non-admins) and returns ok=false; callers should simply return.
//
// If the admin still has the must_change_password flag set, the gate sends
// them to /auth/change-password so the forced-reset flow cannot be skipped
// by deep-linking to an admin page.
//
// Later admin phases (products, categories, orders, ...) reuse this gate as
// their first line.
func (handler httpHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := handler.currentCustomerID(r)
	if email == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return "", false
	}

	isAdmin, err := handler.authSrv.IsAdmin(r.Context(), email)
	if err != nil || !isAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return "", false
	}

	// Force the password change before any admin work. A lookup error here
	// is treated as "not flagged" so a transient DB hiccup doesn't lock
	// the admin out of the panel.
	if must, mcpErr := handler.authSrv.MustChangePassword(r.Context(), email); mcpErr == nil && must {
		http.Redirect(w, r, "/auth/change-password", http.StatusSeeOther)
		return "", false
	}

	return email, true
}

// isAdmin is a defensive helper for templates: it reports whether the current
// request belongs to a logged-in admin, swallowing any lookup error as a
// non-admin. Used by renderTemplate to expose `.IsAdmin`.
func (handler httpHandler) isAdmin(r *http.Request) bool {
	email := handler.currentCustomerID(r)
	if email == "" {
		return false
	}
	isAdmin, err := handler.authSrv.IsAdmin(r.Context(), email)
	return err == nil && isAdmin
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
