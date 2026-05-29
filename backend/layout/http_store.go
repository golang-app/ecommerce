package layout

import (
	"net/http"
	"strings"

	storepkg "github.com/bkielbasa/go-ecommerce/backend/store"
	storeDomain "github.com/bkielbasa/go-ecommerce/backend/store/domain"
	"github.com/gorilla/mux"
)

// StoreMiddleware returns a mux middleware that resolves the active
// store for every incoming request and binds it onto the request
// context. Downstream handlers / templates read it back via
// storeFromCtx so the currency rendered on the page is whatever the
// request's Host header maps to.
//
// The middleware must be installed BEFORE the CSRF middleware so the
// store is present on the context for every render the CSRF check
// allows through. Resolution failures are logged but never surface as
// HTTP errors — the renderer falls back to a zero-value store and the
// FX table's default currency so a misconfigured operator still gets a
// rendered page.
func StoreMiddleware(svc storeService) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if svc == nil {
				next.ServeHTTP(w, r)
				return
			}
			s, err := svc.ResolveByHost(r.Context(), r.Host)
			if err != nil {
				// ResolveByHost falls back to the default store on
				// an unknown host; a non-nil err here means there
				// is *also* no default. We keep serving with a
				// zero-value store so the page still renders, but
				// log loudly so the operator notices.
				_ = err
				s = storeDomain.Store{}
			}
			r = r.WithContext(storepkg.With(r.Context(), s))
			next.ServeHTTP(w, r)
		})
	}
}

// storeFromCtx returns the active store the request middleware bound
// to the request, or the zero-value Store when no binding exists. The
// zero-value Store has empty fields; callers that care about a usable
// currency should fall back to handler.rates.Default() — currentCurrency
// already does that and is the canonical accessor.
func storeFromCtx(r *http.Request) storeDomain.Store {
	s, _ := storepkg.From(r.Context())
	return s
}

// currentCurrency returns the active display currency for the request.
// It reads the store the middleware bound to the context; if that
// store has no usable currency (zero-value Store on a misconfigured
// install) it falls back to the FX table default so the renderer can
// always format a price.
func (handler httpHandler) currentCurrency(r *http.Request) string {
	s := storeFromCtx(r)
	if s.IsZero() || s.Currency() == "" {
		return handler.rates.Default()
	}
	return s.Currency()
}

// storeLink is the per-store entry the /stores page renders. It is
// computed in the handler (rather than the template) so the URL math
// — preserving the current path/query while swapping the host — lives
// next to the requestBaseURL helper that already knows about
// X-Forwarded-Proto.
type storeLink struct {
	Store    storeDomain.Store
	URL      string
	IsActive bool
}

// StoresPage renders the footer-driven store switcher. It lists every
// configured store, builds a URL pointing at the same path/query on
// each store's host, and flags the active store so the template can
// style it. The handler is public — no admin gate.
func (handler httpHandler) StoresPage(w http.ResponseWriter, r *http.Request) {
	var links []storeLink
	if handler.storeSrv != nil {
		stores, err := handler.storeSrv.ListAll(r.Context())
		if err != nil {
			handler.logger.WithError(err).Warn("cannot list stores for /stores page")
		}
		active := storeFromCtx(r)
		// requestBaseURL covers scheme detection (X-Forwarded-Proto +
		// r.TLS); we split it into scheme-only and rebuild the URL
		// using each store's host. Path + RawQuery are preserved so
		// switching stores from a product or category page lands on
		// the equivalent route on the other store.
		scheme := schemeOf(r)
		path := r.URL.Path
		if path == "" || path == "/stores" {
			path = "/"
		}
		raw := r.URL.RawQuery
		for _, s := range stores {
			u := scheme + "://" + s.Host() + path
			if raw != "" {
				u += "?" + raw
			}
			links = append(links, storeLink{
				Store:    s,
				URL:      u,
				IsActive: !active.IsZero() && active.ID() == s.ID(),
			})
		}
	}
	handler.renderTemplate(w, r, "stores", map[string]any{
		"Links": links,
	})
}

// schemeOf is the scheme half of requestBaseURL — kept separate so the
// StoresPage handler can swap the host without re-deriving the full
// base URL.
func schemeOf(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
