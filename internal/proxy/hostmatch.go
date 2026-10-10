package proxy

import (
	"net"
	"strings"
)

// normalizeDomain returns the canonical key for a route domain or a request
// Host header: lowercase, without port, IPv6 brackets or trailing dot. Caddy's
// host matcher is case-insensitive and ignores the port, so the per-domain
// registries must be too.
func normalizeDomain(s string) string {
	s = strings.TrimSpace(s)
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	} else {
		s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	}
	return strings.TrimSuffix(strings.ToLower(s), ".")
}

// hostMatches reports whether a normalized host matches a normalized route
// domain using Caddy's host matcher rules: without "*" it is an exact match;
// otherwise both must have the same number of labels and every "*" label
// matches any single label.
func hostMatches(pattern, host string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == host
	}
	pp := strings.Split(pattern, ".")
	hp := strings.Split(host, ".")
	if len(pp) != len(hp) {
		return false
	}
	for i := range pp {
		if pp[i] != "*" && pp[i] != hp[i] {
			return false
		}
	}
	return true
}

// domainLess orders route domains most specific first: exact hosts, then
// wildcards with fewer "*" labels, then lexically. orderRoutes emits Caddy
// routes in this order, so the first matching domain is the one Caddy serves.
func domainLess(a, b string) bool {
	if ca, cb := strings.Count(a, "*"), strings.Count(b, "*"); ca != cb {
		return ca < cb
	}
	return a < b
}

// lookupHost returns the value registered for the domain Caddy would route
// host to: an exact key first, otherwise the first matching wildcard in
// domainLess order. Keys must be normalized.
func lookupHost[V any](m map[string]V, host string) (V, bool) {
	host = normalizeDomain(host)
	if v, ok := m[host]; ok {
		return v, true
	}
	best, found := "", false
	for k := range m {
		if strings.Contains(k, "*") && hostMatches(k, host) && (!found || domainLess(k, best)) {
			best, found = k, true
		}
	}
	if !found {
		var zero V
		return zero, false
	}
	return m[best], true
}
