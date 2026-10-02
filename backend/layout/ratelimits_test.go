package layout

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP_UntrustedProxy_IgnoresXForwardedFor(t *testing.T) {
	h := httpHandler{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Untrusted public IP
	req.RemoteAddr = "203.0.113.195:43210"
	req.Header.Set("X-Forwarded-For", "198.51.100.5")

	ip := h.clientIP(req)
	if ip != "203.0.113.195" {
		t.Fatalf("expected untrusted proxy X-Forwarded-For to be ignored, got %q, want %q", ip, "203.0.113.195")
	}
}

func TestClientIP_TrustedProxy_HonorsXForwardedFor(t *testing.T) {
	h := httpHandler{}
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		expectedIP string
	}{
		{
			name:       "IPv4 loopback",
			remoteAddr: "127.0.0.1:54321",
			xff:        "198.51.100.5",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "IPv6 loopback",
			remoteAddr: "[::1]:54321",
			xff:        "198.51.100.5",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "RFC 1918 10.x",
			remoteAddr: "10.0.1.15:54321",
			xff:        "198.51.100.5",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "RFC 1918 172.16.x",
			remoteAddr: "172.20.0.2:54321",
			xff:        "198.51.100.5",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "RFC 1918 192.168.x",
			remoteAddr: "192.168.1.100:54321",
			xff:        "198.51.100.5",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "multiple comma-separated IPs in XFF",
			remoteAddr: "10.0.1.15:54321",
			xff:        "198.51.100.5, 10.0.1.20, 10.0.1.30",
			expectedIP: "198.51.100.5",
		},
		{
			name:       "invalid IP in XFF falls back to remote host",
			remoteAddr: "10.0.1.15:54321",
			xff:        "malicious-garbage-string",
			expectedIP: "10.0.1.15",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			req.Header.Set("X-Forwarded-For", tc.xff)

			ip := h.clientIP(req)
			if ip != tc.expectedIP {
				t.Errorf("got %q, want %q", ip, tc.expectedIP)
			}
		})
	}
}

func TestTrustedProxies_CustomCIDR(t *testing.T) {
	h := httpHandler{
		trustedProxies: parseTrustedProxies("198.51.100.0/24"),
	}

	// 10.0.0.1 is no longer trusted because custom list overrides defaults
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	req1.Header.Set("X-Forwarded-For", "203.0.113.1")
	if ip := h.clientIP(req1); ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1 to be untrusted, got %q", ip)
	}

	// 198.51.100.50 IS trusted
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "198.51.100.50:12345"
	req2.Header.Set("X-Forwarded-For", "203.0.113.1")
	if ip := h.clientIP(req2); ip != "203.0.113.1" {
		t.Errorf("expected 198.51.100.50 to be trusted, got %q", ip)
	}
}

func TestParseCIDROrIP_Invalid(t *testing.T) {
	_, err := parseCIDROrIP("invalid-cidr-string")
	if err == nil {
		t.Fatalf("expected error for invalid CIDR, got nil")
	}
}
