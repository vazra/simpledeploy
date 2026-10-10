package proxy

import (
	"maps"
	"net"
	"net/http"
	"sync"

	caddy "github.com/caddyserver/caddy/v2"
	caddyhttp "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// IPAccessRules is the package-level registry used by the Caddy handler.
var IPAccessRules = newIPAccessRegistry()

type parsedAllowlist struct {
	ips  []net.IP
	nets []*net.IPNet
}

// ipAccessRegistry maps normalized route domains to their parsed allowlists.
type ipAccessRegistry struct {
	mu    sync.RWMutex
	rules map[string]*parsedAllowlist
}

func newIPAccessRegistry() *ipAccessRegistry {
	return &ipAccessRegistry{rules: make(map[string]*parsedAllowlist)}
}

// parseAllowlist pre-parses entries into net.IP and net.IPNet for fast
// lookup. Invalid entries are dropped.
func parseAllowlist(entries []string) *parsedAllowlist {
	parsed := &parsedAllowlist{}
	for _, entry := range entries {
		if ip := net.ParseIP(entry); ip != nil {
			parsed.ips = append(parsed.ips, ip)
			continue
		}
		if _, ipNet, err := net.ParseCIDR(entry); err == nil {
			parsed.nets = append(parsed.nets, ipNet)
		}
	}
	return parsed
}

// Set registers or replaces the allowlist for a domain (case-insensitive;
// wildcard domains such as "*.example.com" are allowed).
func (reg *ipAccessRegistry) Set(domain string, entries []string) {
	parsed := parseAllowlist(entries)
	reg.mu.Lock()
	reg.rules[normalizeDomain(domain)] = parsed
	reg.mu.Unlock()
}

// Remove deletes the allowlist for a domain.
func (reg *ipAccessRegistry) Remove(domain string) {
	reg.mu.Lock()
	delete(reg.rules, normalizeDomain(domain))
	reg.mu.Unlock()
}

// Replace atomically swaps the whole rule set for rules (domain -> entries).
// Domains not in rules lose their allowlist.
func (reg *ipAccessRegistry) Replace(rules map[string][]string) {
	next := make(map[string]*parsedAllowlist, len(rules))
	for domain, entries := range rules {
		next[normalizeDomain(domain)] = parseAllowlist(entries)
	}
	reg.mu.Lock()
	reg.rules = next
	reg.mu.Unlock()
}

// snapshot returns a copy of the current rule set.
func (reg *ipAccessRegistry) snapshot() map[string]*parsedAllowlist {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	return maps.Clone(reg.rules)
}

// restore swaps in a rule set taken by snapshot.
func (reg *ipAccessRegistry) restore(rules map[string]*parsedAllowlist) {
	if rules == nil {
		rules = make(map[string]*parsedAllowlist)
	}
	reg.mu.Lock()
	reg.rules = rules
	reg.mu.Unlock()
}

// Allowed returns true if the request should be allowed for host (a request
// Host header; port and case are ignored). host is matched against the
// registered domains the way Caddy's host matcher routes it: exact domain
// first, then the most specific wildcard.
// Returns true when no rules are configured or the allowlist is empty.
func (reg *ipAccessRegistry) Allowed(host string, r *http.Request) bool {
	reg.mu.RLock()
	al, ok := lookupHost(reg.rules, host)
	reg.mu.RUnlock()
	return !ok || al.allows(r)
}

// AllowedFor is like Allowed but uses the rules registered for exactly the
// route domain (no wildcard resolution). Used by handlers bound to a route.
func (reg *ipAccessRegistry) AllowedFor(domain string, r *http.Request) bool {
	reg.mu.RLock()
	al, ok := reg.rules[normalizeDomain(domain)]
	reg.mu.RUnlock()
	return !ok || al.allows(r)
}

func (al *parsedAllowlist) allows(r *http.Request) bool {
	// Empty allowlist = no restriction
	if len(al.ips) == 0 && len(al.nets) == 0 {
		return true
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	clientIP := net.ParseIP(host)
	if clientIP == nil {
		return false
	}

	for _, ip := range al.ips {
		if ip.Equal(clientIP) {
			return true
		}
	}
	for _, ipNet := range al.nets {
		if ipNet.Contains(clientIP) {
			return true
		}
	}
	return false
}

// --- Caddy module ---

func init() {
	caddy.RegisterModule(IPAccessHandler{})
}

// IPAccessHandler is a Caddy middleware that enforces per-domain IP allowlists.
type IPAccessHandler struct {
	// Domain is the route domain whose allowlist applies. buildConfig sets
	// it so the handler uses its own route's rules even when several route
	// domains (e.g. two wildcards) match the request host. Empty falls back
	// to resolving the request Host against the registry.
	Domain string `json:"domain,omitempty"`
}

func (IPAccessHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_ipaccess",
		New: func() caddy.Module { return new(IPAccessHandler) },
	}
}

func (h *IPAccessHandler) Provision(_ caddy.Context) error { return nil }
func (h *IPAccessHandler) Validate() error                 { return nil }

func (h *IPAccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	var allowed bool
	if h.Domain != "" {
		allowed = IPAccessRules.AllowedFor(h.Domain, r)
	} else {
		allowed = IPAccessRules.Allowed(r.Host, r)
	}
	if !allowed {
		http.NotFound(w, r)
		return nil
	}
	return next.ServeHTTP(w, r)
}
