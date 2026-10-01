package layout

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

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
//   - checkoutLimiter: 5 checkout submissions / minute / IP. Protects against
//     automated carding attacks and order flooding.
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
	// checkoutLimiter protects the checkout submission against automated
	// payment attempts and order flooding (5 attempts per minute per IP).
	checkoutLimiter = ratelimit.New(5.0/60.0, 5)
)

// ResetRateLimiters resets all rate limiters to fresh state. Primarily used in
// tests to prevent bucket exhaustion across test suites.
func ResetRateLimiters() {
	loginLimiter = ratelimit.New(5.0/60.0, 5)
	registerLimiter = ratelimit.New(3.0/3600.0, 3)
	addToCartLimiter = ratelimit.New(30.0/60.0, 30)
	forgotPasswordLimiter = ratelimit.New(3.0/3600.0, 3)
	checkoutLimiter = ratelimit.New(5.0/60.0, 5)
}

var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"::1/128",        // IPv6 loopback
	"10.0.0.0/8",     // RFC 1918 private
	"172.16.0.0/12",  // RFC 1918 private
	"192.168.0.0/16", // RFC 1918 private
	"169.254.0.0/16", // RFC 3927 IPv4 link-local
	"fc00::/7",       // RFC 4193 IPv6 unique local
	"fe80::/10",      // RFC 4291 IPv6 link-local
}

var (
	trustedMu      sync.RWMutex
	trustedProxies []*net.IPNet
)

func init() {
	ResetTrustedProxies()
}

// ResetTrustedProxies resets trusted proxy CIDRs to the defaults
// (loopback + RFC1918/RFC4193 private ranges, or TRUSTED_PROXIES env var if set).
func ResetTrustedProxies() {
	trustedMu.Lock()
	defer trustedMu.Unlock()

	envVal := os.Getenv("TRUSTED_PROXIES")
	if envVal != "" {
		if strings.EqualFold(envVal, "none") {
			trustedProxies = nil
			return
		}
		raw := strings.Split(envVal, ",")
		var parsed []*net.IPNet
		for _, entry := range raw {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if n, err := parseCIDROrIP(entry); err == nil {
				parsed = append(parsed, n)
			}
		}
		trustedProxies = parsed
		return
	}

	var parsed []*net.IPNet
	for _, cidr := range defaultTrustedProxyCIDRs {
		if n, err := parseCIDROrIP(cidr); err == nil {
			parsed = append(parsed, n)
		}
	}
	trustedProxies = parsed
}

// SetTrustedProxies configures custom CIDR blocks or IPs whose direct connections
// are trusted as reverse proxies. Passing nil or an empty slice disables X-Forwarded-For trust.
func SetTrustedProxies(cidrs []string) error {
	trustedMu.Lock()
	defer trustedMu.Unlock()

	var parsed []*net.IPNet
	for _, entry := range cidrs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		n, err := parseCIDROrIP(entry)
		if err != nil {
			return err
		}
		parsed = append(parsed, n)
	}
	trustedProxies = parsed
	return nil
}

func parseCIDROrIP(s string) (*net.IPNet, error) {
	if !strings.Contains(s, "/") {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", s)
		}
		if ip.To4() != nil {
			s += "/32"
		} else {
			s += "/128"
		}
	}
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", s, err)
	}
	return ipNet, nil
}

func isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	trustedMu.RLock()
	defer trustedMu.RUnlock()

	for _, network := range trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns a best-effort source IP for rate-limiting decisions.
// The X-Forwarded-For header is ONLY trusted if the direct socket connection
// (r.RemoteAddr) originates from a trusted reverse proxy (e.g. AWS ALB,
// Cloudflare, private VPC, or local loopback). If the socket connection is
// untrusted, X-Forwarded-For is ignored and the host from r.RemoteAddr is returned
// to prevent rate limit spoofing/bypassing.
//
// The string is normalized (trimmed, lower-cased) so two requests from the same
// source always hash to the same bucket key.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr without a port (rare, but documented as
		// possible for non-TCP transports) — use the whole string.
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)

	if isTrustedProxy(r.RemoteAddr) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				xff = xff[:i]
			}
			ip := strings.TrimSpace(xff)
			if parsed := net.ParseIP(ip); parsed != nil {
				return strings.ToLower(parsed.String())
			}
		}
	}

	if host == "" {
		return "unknown"
	}
	return strings.ToLower(host)
}
