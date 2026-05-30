package layout

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// csrfTokenKey is the session-values key under which the per-session CSRF
// token is stored. Using the gorilla/sessions store keeps the token bound
// to the same authenticated cookie the rest of the app already trusts.
const csrfTokenKey = "csrf_token"

// csrfHeader is the request header HTMX clients (and any JS caller) send
// the CSRF token in. Form posts may instead include a hidden input named
// csrfFormField; the middleware accepts whichever arrives.
const csrfHeader = "X-CSRF-Token"

// csrfFormField is the hidden-input name every server-rendered <form
// method="post"> embeds. Standard browser form submits do not have a way
// to set arbitrary headers, so the form value is the fallback channel.
const csrfFormField = "csrf_token"

// csrfTokenBytes is the size, in raw bytes, of a freshly minted CSRF
// token. 32 bytes (256 bits) is generous overkill for a random token whose
// only adversary is an attacker guessing it in a single request.
const csrfTokenBytes = 32

// csrfEnabled is the package-level switch that lets the dev config disable
// CSRF for low-friction local hacking. Production defaults true via the
// CSRFEnabled conf knob. The middleware checks this once per request.
var csrfEnabled = true

// setCSRFEnabled is called from layout.New() to wire the config knob into
// the package-level switch. Keeping it package-scoped (rather than passing
// it down through every handler) matches the existing `store` pattern.
func setCSRFEnabled(enabled bool) {
	csrfEnabled = enabled
}

// issueCSRFToken returns the CSRF token for the current session, minting a
// new one (and persisting it back via Save) when the session does not yet
// carry one. Callers must invoke it before the response body has been
// committed, since session.Save sets a Set-Cookie header.
func issueCSRFToken(r *http.Request, w http.ResponseWriter) (string, error) {
	session, err := store.Get(r, "ecommerce")
	if err != nil {
		// gorilla returns a usable (new) session even on cookie-decode
		// errors, so we keep going rather than failing the render.
		session, _ = store.New(r, "ecommerce")
	}

	if existing, ok := session.Values[csrfTokenKey].(string); ok && existing != "" {
		return existing, nil
	}

	buf := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(buf)
	session.Values[csrfTokenKey] = tok
	if err := session.Save(r, w); err != nil {
		return "", err
	}
	return tok, nil
}

// sessionCSRFToken reads the current CSRF token off the session without
// minting a new one. It returns "" when the session has no token yet, in
// which case the request is rejected (the form was rendered without a
// token, which should be impossible for any legitimate client).
func sessionCSRFToken(r *http.Request) string {
	session, err := store.Get(r, "ecommerce")
	if err != nil {
		return ""
	}
	tok, _ := session.Values[csrfTokenKey].(string)
	return tok
}

// CSRFMiddleware is the exported entry point main.go uses to install the
// CSRF check on the application-wide router. It is a thin wrapper around
// the package-internal middleware so callers outside the package do not
// have to know about the implementation symbol.
func CSRFMiddleware(next http.Handler) http.Handler {
	return csrfMiddleware(next)
}

// csrfMiddleware enforces the double-submit check on every unsafe request
// (POST/PUT/PATCH/DELETE). Safe methods are forwarded unchanged so the
// template-rendering path that mints the token is itself never blocked.
//
// The token may arrive via the X-CSRF-Token header (used by HTMX, set
// uniformly by the configRequest script in the base layout) or via the
// csrf_token hidden form field (used by standard browser form posts). We
// prefer the header to avoid consuming the request body when the handler
// downstream wants raw access to it; only if the header is empty do we
// fall back to ParseForm/ParseMultipartForm, both of which are documented
// as idempotent on net/http so a downstream re-parse is safe.
func csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !csrfEnabled || isSafeMethod(r.Method) || isWebhookPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		expected := sessionCSRFToken(r)
		if expected == "" {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		got := r.Header.Get(csrfHeader)
		if got == "" {
			// Parse the body so the handler downstream sees the
			// same parsed form. ParseForm / ParseMultipartForm are
			// both idempotent on a *http.Request.
			ct := r.Header.Get("Content-Type")
			if strings.HasPrefix(ct, "multipart/form-data") {
				// 32 MiB matches net/http's default. The
				// disk-image handlers downstream call
				// ParseMultipartForm themselves with the
				// same default, so we don't shrink it here.
				_ = r.ParseMultipartForm(32 << 20)
			} else {
				_ = r.ParseForm()
			}
			got = r.FormValue(csrfFormField)
		}

		if got == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(got)) != 1 {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isSafeMethod returns true for HTTP methods that are spec-defined as
// state-changing-free and therefore exempt from CSRF enforcement.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// isWebhookPath returns true for inbound webhook routes. These come
// from outside the browser (e.g. a payment provider's webhook
// emitter), carry no session cookie, and are authenticated by a
// signed body — they have no CSRF token to present, and demanding
// one would block every legitimate delivery. The handler downstream
// (see http_payments_webhook.go) verifies the signature before
// touching state, so the bypass is safe.
func isWebhookPath(path string) bool {
	return strings.HasPrefix(path, "/webhooks/")
}
