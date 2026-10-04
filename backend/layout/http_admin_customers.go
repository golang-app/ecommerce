package layout

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/gorilla/mux"
)

type adminCustomerListItem struct {
	Email              string
	Name               string
	ShipName           string
	Type               string // "registered" or "guest"
	IsRegistered       bool
	OrderCount         int
	TotalSpent         int64
	TotalSpentDisplay  string
	TotalSpentCurrency string
	LastOrderAt        time.Time
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// AdminCustomers handles GET /admin/customers.
// It lists all unique customers across registered accounts (auth service)
// and guest purchasers (checkout order stats), supporting filter and search.
func (handler httpHandler) AdminCustomers(w http.ResponseWriter, r *http.Request) {
	adminEmail, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	regEmails, err := handler.authSrv.ListCustomers(r.Context())
	if err != nil {
		handler.logger.WithError(err).Error("cannot list registered customers")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	orderStats, err := handler.checkoutQry.ListCustomerOrderStats(r.Context())
	if err != nil {
		handler.logger.WithError(err).Error("cannot list customer order stats")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	customerMap := make(map[string]*adminCustomerListItem)

	// Populate entries from checkout order stats (includes both registered users and guest purchasers)
	for _, stat := range orderStats {
		emailKey := strings.ToLower(strings.TrimSpace(stat.CustomerID()))
		currency := stat.Currency()
		if currency == "" {
			currency = "USD"
		}
		customerMap[emailKey] = &adminCustomerListItem{
			Email:              stat.CustomerID(),
			Name:               stat.LatestShipName(),
			ShipName:           stat.LatestShipName(),
			Type:               "guest",
			IsRegistered:       false,
			OrderCount:         stat.OrderCount(),
			TotalSpent:         stat.TotalSpent(),
			TotalSpentDisplay:  formatCents(stat.TotalSpent()),
			TotalSpentCurrency: currency,
			LastOrderAt:        stat.LastOrderAt(),
		}
	}

	// Merge registered customer emails (marks matching orders as registered, adds registered users with 0 orders)
	for _, regEmail := range regEmails {
		emailKey := strings.ToLower(strings.TrimSpace(regEmail))
		if item, exists := customerMap[emailKey]; exists {
			item.IsRegistered = true
			item.Type = "registered"
		} else {
			customerMap[emailKey] = &adminCustomerListItem{
				Email:              regEmail,
				Type:               "registered",
				IsRegistered:       true,
				OrderCount:         0,
				TotalSpent:         0,
				TotalSpentDisplay:  "0.00",
				TotalSpentCurrency: "USD",
			}
		}
	}

	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("filter")))
	search := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	var filtered []*adminCustomerListItem
	for _, item := range customerMap {
		if filter == "registered" && !item.IsRegistered {
			continue
		}
		if filter == "guest" && item.IsRegistered {
			continue
		}

		if search != "" {
			matchEmail := strings.Contains(strings.ToLower(item.Email), search)
			matchName := strings.Contains(strings.ToLower(item.Name), search)
			matchShipName := strings.Contains(strings.ToLower(item.ShipName), search)
			if !matchEmail && !matchName && !matchShipName {
				continue
			}
		}

		filtered = append(filtered, item)
	}

	// Sort by LastOrderAt desc (most recent first; customers without orders last), then by Email asc
	sort.Slice(filtered, func(i, j int) bool {
		iZero := filtered[i].LastOrderAt.IsZero()
		jZero := filtered[j].LastOrderAt.IsZero()
		if iZero != jZero {
			return !iZero
		}
		if !filtered[i].LastOrderAt.Equal(filtered[j].LastOrderAt) {
			return filtered[i].LastOrderAt.After(filtered[j].LastOrderAt)
		}
		return filtered[i].Email < filtered[j].Email
	})

	handler.renderAdminTemplate(w, r, "admin/customers", map[string]any{
		"Active":    "customers",
		"Email":     adminEmail,
		"Customers": filtered,
		"Filter":    filter,
		"Q":         r.URL.Query().Get("q"),
		"Query":     r.URL.Query().Get("q"),
	})
}

// AdminCustomerDetail handles GET /admin/customers/{email}.
// It displays a customer's profile, lifetime metrics, order history,
// saved shipping addresses, submitted reviews, and saved wishlist items.
func (handler httpHandler) AdminCustomerDetail(w http.ResponseWriter, r *http.Request) {
	adminEmail, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	customerEmail := mux.Vars(r)["email"]
	if customerEmail == "" {
		http.Redirect(w, r, "/admin/customers", http.StatusSeeOther)
		return
	}

	// 1. Account status: verify whether email is registered
	isRegistered, err := handler.authSrv.IsRegistered(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Error("cannot check if customer is registered")
	}

	// 2. Order history
	orders, err := handler.checkoutQry.ListByCustomer(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Error("cannot list customer orders")
		https.InternalError(w, "internal-error", err.Error())
		return
	}

	// 3. Saved shipping addresses
	addresses, err := handler.shipSrv.List(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot list customer shipping addresses")
		addresses = nil
	}

	// 4. Product reviews
	reviews, err := handler.reviewsSrv.ListByCustomer(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot list customer reviews")
		reviews = nil
	}

	// 5. Wishlist bookmarks
	wishlist, err := handler.wishlistSrv.ListByCustomer(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot list customer wishlist")
		wishlist = nil
	}

	// Derive customer display name from shipping address or order stats
	customerName := ""
	for _, a := range addresses {
		if a.IsDefault() && a.Name() != "" {
			customerName = a.Name()
			break
		}
		if customerName == "" && a.Name() != "" {
			customerName = a.Name()
		}
	}
	if customerName == "" {
		if stats, err := handler.checkoutQry.ListCustomerOrderStats(r.Context()); err == nil {
			for _, s := range stats {
				if strings.EqualFold(strings.TrimSpace(s.CustomerID()), strings.TrimSpace(customerEmail)) && s.LatestShipName() != "" {
					customerName = s.LatestShipName()
					break
				}
			}
		}
	}

	// Calculate lifetime order stats & AOV
	var totalSpent int64
	currency := "USD"
	for _, o := range orders {
		totalSpent += o.TotalAmount()
		if o.TotalCurrency() != "" {
			currency = o.TotalCurrency()
		}
	}
	var aov int64
	if len(orders) > 0 {
		aov = totalSpent / int64(len(orders))
	}

	customerType := "guest"
	if isRegistered {
		customerType = "registered"
	}

	customerView := map[string]any{
		"Email":              customerEmail,
		"Name":               customerName,
		"ShipName":           customerName,
		"Type":               customerType,
		"IsRegistered":       isRegistered,
		"OrderCount":         len(orders),
		"TotalSpent":         totalSpent,
		"TotalSpentDisplay":  formatCents(totalSpent),
		"TotalSpentCurrency": currency,
	}

	statsView := map[string]any{
		"TotalOrders":        len(orders),
		"TotalSpent":         totalSpent,
		"TotalSpentDisplay":  formatCents(totalSpent),
		"TotalSpentCurrency": currency,
		"AOV":                aov,
		"AOVDisplay":         formatCents(aov),
		"AOVCurrency":        currency,
	}

	handler.renderAdminTemplate(w, r, "admin/customer_detail", map[string]any{
		"Active":       "customers",
		"Email":        adminEmail,
		"Customer":     customerView,
		"Stats":        statsView,
		"Orders":       orders,
		"Addresses":    addresses,
		"Reviews":      reviews,
		"Wishlist":     wishlist,
		"Name":         customerName,
		"ShipName":     customerName,
		"IsRegistered": isRegistered,
		"Type":         customerType,
	})
}

// AdminCustomerTriggerPasswordReset handles POST /admin/customers/{email}/reset-password.
// Only registered customers can have a password reset triggered. For guest customers,
// it sets an error flash message and redirects back without generating a token or sending email.
func (handler httpHandler) AdminCustomerTriggerPasswordReset(w http.ResponseWriter, r *http.Request) {
	_, ok := handler.requireAdmin(w, r)
	if !ok {
		return
	}

	customerEmail := mux.Vars(r)["email"]
	if customerEmail == "" {
		http.Redirect(w, r, "/admin/customers", http.StatusSeeOther)
		return
	}

	// Verify customer is registered
	isRegistered, err := handler.authSrv.IsRegistered(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Error("cannot check if customer is registered for password reset")
		handler.flash(w, r, "Failed to verify customer", "error")
		http.Redirect(w, r, "/admin/customers/"+customerEmail, http.StatusSeeOther)
		return
	}

	if !isRegistered {
		handler.flash(w, r, "Cannot reset password for guest customer", "error")
		http.Redirect(w, r, "/admin/customers/"+customerEmail, http.StatusSeeOther)
		return
	}

	rawToken, err := handler.authSrv.RequestPasswordReset(r.Context(), customerEmail)
	if err != nil {
		handler.logger.WithError(err).Error("cannot request password reset")
		handler.flash(w, r, "Failed to initiate password reset", "error")
		http.Redirect(w, r, "/admin/customers/"+customerEmail, http.StatusSeeOther)
		return
	}

	if rawToken != "" {
		msg, rerr := RenderPasswordReset(customerEmail, rawToken, handler.baseURL, passwordResetTTLMinutes)
		if rerr != nil {
			handler.logger.WithError(rerr).Error("cannot render password reset email")
		} else if handler.mailer != nil {
			if serr := handler.mailer.Send(r.Context(), msg); serr != nil {
				handler.logger.WithError(serr).Error("cannot send password reset email")
			}
		}
	}

	handler.flash(w, r, "Password reset email sent to "+customerEmail, "")
	http.Redirect(w, r, "/admin/customers/"+customerEmail, http.StatusSeeOther)
}
