package proxy

import (
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

// ipAccessRegistry maps domains to their parsed allowlists.
type ipAccessRegistry struct {
	mu    sync.RWMutex
	rules map[string]*parsedAllowlist
}

func newIPAccessRegistry() *ipAccessRegistry {
	return &ipAccessRegistry{rules: make(map[string]*parsedAllowlist)}
}

// Set registers or replaces the allowlist for a domain.
// Entries are pre-parsed into net.IP and net.IPNet for fast lookup.
func (reg *ipAccessRegistry) Set(domain string, entries []string) {
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
	reg.mu.Lock()
	reg.rules[domain] = parsed
	reg.mu.Unlock()
}

// Remove deletes the allowlist for a domain.
func (reg *ipAccessRegistry) Remove(domain string) {
	reg.mu.Lock()
	delete(reg.rules, domain)
	reg.mu.Unlock()
}

// Allowed returns true if the request should be allowed for the domain.
// Returns true when no rules are configured or the allowlist is empty.
func (reg *ipAccessRegistry) Allowed(domain string, r *http.Request) bool {
	reg.mu.RLock()
	al, ok := reg.rules[domain]
	reg.mu.RUnlock()

	if !ok {
		return true
	}
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
type IPAccessHandler struct{}

func (IPAccessHandler) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.simpledeploy_ipaccess",
		New: func() caddy.Module { return new(IPAccessHandler) },
	}
}

func (h *IPAccessHandler) Provision(_ caddy.Context) error { return nil }
func (h *IPAccessHandler) Validate() error                 { return nil }

func (h *IPAccessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	host := r.Host
	if h2, _, err := net.SplitHostPort(host); err == nil && h2 != "" {
		host = h2
	}
	if !IPAccessRules.Allowed(host, r) {
		http.NotFound(w, r)
		return nil
	}
	return next.ServeHTTP(w, r)
}
