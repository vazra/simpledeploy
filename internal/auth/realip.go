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

func isTrusted(ip string, trusted []string) bool {
	for _, t := range trusted {
		if ip == t {
			return true
		}
	}
	return false
}
