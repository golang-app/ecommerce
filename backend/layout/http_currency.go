package layout

import (
	"html"
	"html/template"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
)

// moneyFunc returns the template FuncMap callback bound to the active
// rates table and the request's display currency. It is the one place
// the conversion+format logic lives — both renderTemplate and the
// HTMX-only fragment renderers (e.g. AllProducts) install the same
// helper so {{ money .X }} works uniformly everywhere on the storefront.
//
// The helper assumes the input is a minor-unit amount in rates.Default()
// (USD). Conversion is a render transformation; orders are placed and
// charged in the default currency regardless of what the helper renders.
// The active display currency is now sourced from the request-bound
// Store (see http_store.go currentCurrency) rather than a per-customer
// session preference — a single request always renders the price the
// store it landed on charges.
func moneyFunc(rates fx.Rates, currency string) func(int64) template.HTML {
	return func(minorUSD int64) template.HTML {
		m := fx.Format(rates, minorUSD, currency)
		// Both pieces are escaped before concatenation. Display() is a
		// numeric "X.YY" string and Currency is an ISO code, but
		// passing them through html.EscapeString keeps the helper
		// XSS-safe even if a future call site sneaks in a hand-built
		// string.
		return template.HTML(html.EscapeString(m.Display()) + " " + html.EscapeString(m.Currency))
	}
}
