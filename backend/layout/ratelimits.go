package layout

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// rateLimiter is the narrow internal seam the presentation layer requires
// to check whether a given client action is allowed under the rate limit policy.
type rateLimiter interface {
	Allow(ctx context.Context, action string, key string) bool
}

// allowRate checks if an action by the requesting client is allowed.
// If no limiter is configured on the handler (e.g. in minimal unit test setups),
// requests are allowed by default.
func (h httpHandler) allowRate(ctx context.Context, action string, r *http.Request) bool {
	if h.limiter == nil {
		return true
	}
	return h.limiter.Allow(ctx, action, h.clientIP(r))
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

var defaultTrustedProxies = func() []*net.IPNet {
	var parsed []*net.IPNet
	for _, cidr := range defaultTrustedProxyCIDRs {
		if n, err := parseCIDROrIP(cidr); err == nil {
			parsed = append(parsed, n)
		}
	}
	return parsed
}()

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

func parseTrustedProxies(config string) []*net.IPNet {
	config = strings.TrimSpace(config)
	if config == "" {
		return defaultTrustedProxies
	}
	if strings.EqualFold(config, "none") {
		return []*net.IPNet{}
	}
	raw := strings.Split(config, ",")
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
	return parsed
}

func (h httpHandler) isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	proxies := h.trustedProxies
	if proxies == nil {
		proxies = defaultTrustedProxies
	}

	for _, network := range proxies {
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
func (h httpHandler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)

	if h.isTrustedProxy(r.RemoteAddr) {
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
