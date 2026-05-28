package layout

import (
	"html"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fx"
)

// moneyFunc returns the template FuncMap callback bound to the active
// rates table and the customer's chosen display currency. It is the one
// place the conversion+format logic lives — both renderTemplate and the
// HTMX-only fragment renderers (e.g. AllProducts) install the same
// helper so {{ money .X }} works uniformly everywhere on the storefront.
//
// The helper assumes the input is a minor-unit amount in rates.Default()
// (USD). Conversion is a render transformation; orders are placed and
// charged in the default currency regardless of what the helper renders.
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

// currencySessionKey is the gorilla/sessions key under which the
// customer's chosen display currency is stored. The default cookie
// MaxAge (30 days) is fine — the preference is sticky across visits
// but expires naturally if the shopper goes away.
const currencySessionKey = "currency"

// currentCurrency returns the customer's active display currency. It
// reads the session value first and falls back to the configured
// default when the session has no preference, is unreadable, or carries
// a value that is no longer supported (the operator may have removed a
// currency from SUPPORTED_CURRENCIES since the cookie was set).
func (handler httpHandler) currentCurrency(r *http.Request) string {
	session, err := store.Get(r, "ecommerce")
	if err != nil {
		return handler.rates.Default()
	}
	if v, ok := session.Values[currencySessionKey].(string); ok && v != "" {
		v = strings.ToUpper(v)
		if handler.rates.IsSupported(v) {
			return v
		}
	}
	return handler.rates.Default()
}

// HandleSetCurrency processes the header currency-picker POST. It
// accepts a `currency` form field, validates it against the supported
// list, persists the choice on the session, and redirects the customer
// back to the page they came from (or `/` if no Referer was sent).
//
// CSRF is enforced by the global middleware: the picker form embeds the
// hidden csrf_token input on every render.
func (handler httpHandler) HandleSetCurrency(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	chosen := strings.ToUpper(strings.TrimSpace(r.PostFormValue("currency")))
	if chosen == "" || !handler.rates.IsSupported(chosen) {
		// Unknown / unsupported codes are silently ignored — the
		// picker only renders supported codes, so this only fires
		// when someone is crafting a POST by hand.
		http.Redirect(w, r, safeRedirectTarget(r), http.StatusSeeOther)
		return
	}

	session, err := store.Get(r, "ecommerce")
	if err != nil {
		handler.logger.WithError(err).Warn("currency picker: cannot read session")
		http.Redirect(w, r, safeRedirectTarget(r), http.StatusSeeOther)
		return
	}
	session.Values[currencySessionKey] = chosen
	if err := session.Save(r, w); err != nil {
		handler.logger.WithError(err).Error("currency picker: cannot save session")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, safeRedirectTarget(r), http.StatusSeeOther)
}

// safeRedirectTarget returns the Referer path the picker should send
// the shopper back to. We only accept same-origin Referers (relative
// paths or URLs whose Host matches the request) so an attacker cannot
// turn the picker into an open redirect.
func safeRedirectTarget(r *http.Request) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return "/"
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "/"
	}
	// Same-host (or relative) Referers are safe to send the user back to.
	if u.Host != "" && u.Host != r.Host {
		return "/"
	}
	path := u.RequestURI()
	if path == "" {
		return "/"
	}
	return path
}
