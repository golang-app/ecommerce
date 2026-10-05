package layout

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/sirupsen/logrus"
)

func TestAdminCustomerTemplatesParsing(t *testing.T) {
	is := is.New(t)

	origWd, err := os.Getwd()
	if err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	// Ensure cookie store is initialized for sessions/CSRF
	if store == nil {
		store = newCookieStore([]byte("test-secret-32-bytes-long-123456"), false)
	}

	handler := httpHandler{
		logger: logrus.New(),
	}

	t.Run("admin/customers template", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/customers?q=john&filter=registered", nil)
		// Attach session
		session, _ := store.Get(req, "ecommerce")
		session.Values["admin_email"] = "admin@example.com"
		_ = session.Save(req, rec)

		type customerItem struct {
			Email             string
			Name              string
			Type              string
			IsRegistered      bool
			OrderCount        int
			TotalSpentDisplay string
			TotalSpentCurrency string
			LastOrderAt       time.Time
		}

		data := map[string]any{
			"Active": "customers",
			"Query":  "john",
			"Filter": "registered",
			"Customers": []customerItem{
				{
					Email:              "john@example.com",
					Name:               "John Doe",
					Type:               "registered",
					IsRegistered:       true,
					OrderCount:         3,
					TotalSpentDisplay:  "$150.00",
					TotalSpentCurrency: "USD",
					LastOrderAt:        time.Now(),
				},
				{
					Email:              "guest@example.com",
					Name:               "Guest User",
					Type:               "guest",
					IsRegistered:       false,
					OrderCount:         1,
					TotalSpentDisplay:  "$45.00",
					TotalSpentCurrency: "USD",
					LastOrderAt:        time.Now(),
				},
			},
		}

		handler.renderAdminTemplate(rec, req, "admin/customers", data)
		is.Equal(rec.Code, http.StatusOK)
		is.True(len(rec.Body.String()) > 0)
	})

	t.Run("admin/customer_detail template", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/customers/john@example.com", nil)
		session, _ := store.Get(req, "ecommerce")
		session.Values["admin_email"] = "admin@example.com"
		_ = session.Save(req, rec)

		type customerDetail struct {
			Email             string
			Name              string
			Type              string
			IsRegistered      bool
			OrderCount        int
			TotalSpentDisplay string
		}

		type orderItem struct {
			ID           string
			PlacedAt     time.Time
			Status       string
			ItemCount    int
			TotalDisplay string
			TotalCurrency string
		}

		type addressItem struct {
			Name      string
			Street1   string
			Street2   string
			City      string
			Zip       string
			Country   string
			IsDefault bool
		}

		type reviewItem struct {
			ProductID string
			Rating    int
			Body      string
			Status    string
			CreatedAt time.Time
		}

		type wishItem struct {
			VariantID string
			AddedAt   time.Time
		}

		data := map[string]any{
			"Active": "customers",
			"Customer": customerDetail{
				Email:             "john@example.com",
				Name:              "John Doe",
				Type:              "registered",
				IsRegistered:      true,
				OrderCount:        2,
				TotalSpentDisplay: "$200.00",
			},
			"Stats": map[string]any{
				"TotalOrders":       2,
				"TotalSpentDisplay": "$200.00",
				"TotalSpentCurrency": "USD",
				"AOVDisplay":        "$100.00",
				"AOVCurrency":       "USD",
			},
			"Orders": []orderItem{
				{
					ID:            "ord-1",
					PlacedAt:      time.Now(),
					Status:        "paid",
					ItemCount:     2,
					TotalDisplay:  "$100.00",
					TotalCurrency: "USD",
				},
			},
			"Addresses": []addressItem{
				{
					Name:      "John Doe",
					Street1:   "123 Main St",
					City:      "New York",
					Zip:       "10001",
					Country:   "US",
					IsDefault: true,
				},
			},
			"Reviews": []reviewItem{
				{
					ProductID: "prod-1",
					Rating:    5,
					Body:      "Great product!",
					Status:    "approved",
					CreatedAt: time.Now(),
				},
			},
			"Wishlist": []wishItem{
				{
					VariantID: "var-1",
					AddedAt:   time.Now(),
				},
			},
		}

		handler.renderAdminTemplate(rec, req, "admin/customer_detail", data)
		is.Equal(rec.Code, http.StatusOK)
		is.True(len(rec.Body.String()) > 0)
	})
}

func TestAdminNavSections(t *testing.T) {
	is := is.New(t)

	origWd, err := os.Getwd()
	if err == nil && filepath.Base(origWd) == "layout" {
		if err := os.Chdir(".."); err == nil {
			t.Cleanup(func() { _ = os.Chdir(origWd) })
		}
	}

	if store == nil {
		store = newCookieStore([]byte("test-secret-32-bytes-long-123456"), false)
	}

	handler := httpHandler{
		logger: logrus.New(),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	session, _ := store.Get(req, "ecommerce")
	session.Values["admin_email"] = "admin@example.com"
	_ = session.Save(req, rec)

	handler.renderAdminTemplate(rec, req, "admin/dashboard", map[string]any{
		"Active": "dashboard",
	})
	is.Equal(rec.Code, http.StatusOK)
	body := rec.Body.String()

	// Verify all section headings (case-insensitive check via lowercase HTML text)
	lowerBody := strings.ToLower(body)
	is.True(strings.Contains(lowerBody, "sales"))
	is.True(strings.Contains(lowerBody, "catalog"))
	is.True(strings.Contains(lowerBody, "customers"))
	is.True(strings.Contains(lowerBody, "marketing"))
	is.True(strings.Contains(lowerBody, "configuration"))

	// Verify all 14 routes are present in navigation
	routes := []string{
		"/admin",
		"/admin/orders",
		"/admin/products",
		"/admin/categories",
		"/admin/attributes",
		"/admin/attribute-sets",
		"/admin/inventory",
		"/admin/customers",
		"/admin/reviews",
		"/admin/promo-codes",
		"/admin/repricing",
		"/admin/payment-providers",
		"/admin/shipping-providers",
		"/admin/stores",
	}
	for _, route := range routes {
		is.True(strings.Contains(body, `href="`+route+`"`))
	}
}
