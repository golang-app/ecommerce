package layout

import (
	"net/http"
)

// requireAdmin is the access gate for every admin page. It resolves the
// current customer and confirms they hold the admin flag. On failure it has
// already written the response (a redirect to login for anonymous users, or a
// 403 for non-admins) and returns ok=false; callers should simply return.
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

	handler.renderAdminTemplate(w, r, "admin/dashboard", map[string]any{
		"Active":        "dashboard",
		"Email":         email,
		"ProductCount":  len(products),
		"CategoryCount": len(categories),
		"OrderCount":    len(orders),
	})
}
