package layout

import (
	"net"
	"net/http"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/internal/ratelimit"
)

// Package-level limiters. One process, one set of buckets — keeping them
// here (rather than on httpHandler) avoids threading them through every
// handler signature and matches the existing `store` package var pattern.
//
// Rates are tuned conservatively for the abuse surface, not for legitimate
// repeat traffic:
//
//   - loginLimiter: 5 logins / minute / IP. A human typing the wrong
//     password a handful of times still gets through; credential-stuffing
//     bots do not.
//   - registerLimiter: 3 registrations / hour / IP. Real signups are rare;
//     anything above this rate is almost certainly account-creation abuse.
//   - addToCartLimiter: 30 cart-adds / minute / IP. The storefront triggers
//     this once per click, so legitimate browsing stays well under; rapid
//     scraping that hammers the variant box gets a 429.
var (
	loginLimiter     = ratelimit.New(5.0/60.0, 5)
	registerLimiter  = ratelimit.New(3.0/3600.0, 3)
	addToCartLimiter = ratelimit.New(30.0/60.0, 30)
	// forgotPasswordLimiter caps the password-reset request rate at 3/hour
	// per IP. Reset emails are an asymmetric workload (one cheap form post
	// triggers a full SMTP send), and they double as an enumeration vector
	// (timing differences between "exists" and "doesn't exist" code paths
	// can leak signal); the same per-IP throttle that protects /auth/
	// register fits here for the same reasons.
	forgotPasswordLimiter = ratelimit.New(3.0/3600.0, 3)
)

// clientIP returns a best-effort source IP for rate-limiting decisions.
// The first comma-delimited entry of X-Forwarded-For is honored when
// present (typical for deployments behind a reverse proxy); otherwise we
// fall back to the host portion of r.RemoteAddr. The string is normalized
// (trimmed, lower-cased) so two requests from the same source always hash
// to the same bucket key.
//
// We deliberately do NOT honor X-Real-IP or other arbitrary headers — only
// X-Forwarded-For, which is the convention every proxy in our stack emits.
// In a setup that does not strip these headers at the edge, a malicious
// client could spoof X-Forwarded-For; the protections here are still a
// useful first line, but a hardened deployment should terminate XFF at the
// reverse proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		ip := strings.TrimSpace(xff)
		if ip != "" {
			return strings.ToLower(ip)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr without a port (rare, but documented as
		// possible for non-TCP transports) — use the whole string.
		return strings.ToLower(r.RemoteAddr)
	}
	return strings.ToLower(host)
}
