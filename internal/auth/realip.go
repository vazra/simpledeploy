package auth

import (
	"net"
	"net/http"
	"strings"
)

// RealIP extracts the client IP from a request, checking X-Forwarded-For
// when the direct connection comes from a trusted proxy.
func RealIP(r *http.Request, trustedProxies []string) string {
	directIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		directIP = r.RemoteAddr
	}

	if len(trustedProxies) == 0 {
		return directIP
	}

	if !isTrusted(directIP, trustedProxies) {
		return directIP
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return directIP
	}

	// Parse right-to-left, return last untrusted IP
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if ip == "" {
			continue
		}
		if !isTrusted(ip, trustedProxies) {
			return ip
		}
	}

	return directIP
}

// isTrusted accepts plain IPs ("10.0.0.1") and CIDR ranges ("10.0.0.0/8")
// in trusted. Malformed entries are skipped (not matched).
func isTrusted(ip string, trusted []string) bool {
	parsed := net.ParseIP(ip)
	for _, t := range trusted {
		if t == ip {
			return true
		}
		if parsed != nil && strings.Contains(t, "/") {
			if _, n, err := net.ParseCIDR(t); err == nil && n.Contains(parsed) {
				return true
			}
		}
	}
	return false
}
