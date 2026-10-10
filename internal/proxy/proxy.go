package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	caddy "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

// maxRequestHeaderBytes caps request headers on every proxy listener. Caddy
// defaults to 16 KiB, which refuses apps with large cookies or tokens with
// a 431 that never reaches the app or SimpleDeploy's metrics; keep Go's
// 1 MiB default instead.
const maxRequestHeaderBytes = 1 << 20

// requestBodyIdleTimeout is how long an upload to an HTTP route may stall
// before the connection is closed (slowloris protection). gRPC and h2c
// routes get no idle limit: a client-streaming or bidi RPC can legitimately
// stay quiet for a long time. A variable so tests can shorten it.
var requestBodyIdleTimeout = time.Minute

// Proxy manages reverse-proxy routes.
type Proxy interface {
	SetRoutes(routes []Route) error
	Stop() error
}

// CaddyConfig holds configuration for a CaddyProxy.
type CaddyConfig struct {
	ListenAddr       string   // e.g. ":443"
	HTTPListenAddr   string   // optional HTTP listener for plain-HTTP to HTTPS redirect, e.g. ":80"
	ExtraListenAddrs []string // optional extra listeners for the main server, e.g. [":50051"]; same routes and TLS
	TLSMode          string   // "auto", "custom", "off", "local"
	TLSEmail         string   // ACME email, used when TLSMode is "auto"
	DataDir          string   // data directory for Caddy storage
}

// CaddyProxy is a Proxy backed by Caddy.
type CaddyProxy struct {
	mu               sync.Mutex
	routes           []Route
	listenAddr       string
	extraListenAddrs []string
	httpListenAddr   string
	tlsMode          string
	tlsEmail         string
	dataDir          string
	owners           map[string]string // normalized domain -> app slug serving it; guarded by mu

	// loadMu serializes config loads and whole SetRoutes updates.
	loadMu  sync.Mutex
	lastKey []byte                   // config JSON + cert file fingerprint of the last successful load
	load    func([]byte, bool) error // caddy.Load; replaceable in tests
}

// NewCaddyProxy creates a CaddyProxy from the given config.
func NewCaddyProxy(cfg CaddyConfig) *CaddyProxy {
	return &CaddyProxy{
		load:             caddy.Load,
		listenAddr:       cfg.ListenAddr,
		extraListenAddrs: append([]string(nil), cfg.ExtraListenAddrs...),
		httpListenAddr:   cfg.HTTPListenAddr,
		tlsMode:          cfg.TLSMode,
		tlsEmail:         cfg.TLSEmail,
		dataDir:          cfg.DataDir,
	}
}

var validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.*-]*$`)

// SetRoutes stores routes, configures rate limiters and IP allowlists, and
// reloads Caddy config. Routes with an invalid domain, or on a domain another
// app serves, are dropped with a warning (see claimDomains), so one app's bad
// endpoint cannot block route updates for every other app. If the new config
// fails to load, routes, domain owners and rules revert to the set Caddy is
// still running.
func (c *CaddyProxy) SetRoutes(routes []Route) error {
	valid := make([]Route, 0, len(routes))
	for _, r := range routes {
		if !validDomainRe.MatchString(r.Domain) {
			log.Printf("[proxy] WARNING: skip route of app %q: invalid domain %q", r.AppSlug, r.Domain)
			continue
		}
		valid = append(valid, r)
	}
	routes = valid

	// Held for the whole update so no other load runs between applying the
	// new state and restoring the old one on failure.
	c.loadMu.Lock()
	defer c.loadMu.Unlock()

	// Registries are updated under mu so concurrent readers never see rules
	// from one route set next to routes from another.
	c.mu.Lock()
	prevRoutes, prevOwners := c.routes, c.owners
	prevRules := snapshotRouteRules()
	kept, owners := claimDomains(routes, c.owners)
	c.routes = kept
	c.owners = owners
	registerRouteRules(kept)
	c.mu.Unlock()

	if err := c.reloadLocked(false); err != nil {
		c.mu.Lock()
		c.routes, c.owners = prevRoutes, prevOwners
		prevRules.restore()
		c.mu.Unlock()
		return err
	}
	return nil
}

// claimDomains keeps one app per domain so an app can never take over, or
// clear the IP allowlist / rate limit of, a domain another app serves.
// Domains compare case-insensitively, ignoring a trailing dot. The owner is
// the app that owned the domain in prev if it still claims it, otherwise the
// app with the lowest AppID (slug breaks ties), which is also the rule after
// a restart; the choice never depends on route order. Other apps' routes on that
// domain are dropped with a warning. Returns kept routes in input order and
// the new owner map.
func claimDomains(routes []Route, prev map[string]string) ([]Route, map[string]string) {
	claimants := map[string]map[string]int64{} // domain -> slug -> lowest AppID
	for _, r := range routes {
		key := normalizeDomain(r.Domain)
		apps := claimants[key]
		if apps == nil {
			apps = map[string]int64{}
			claimants[key] = apps
		}
		if id, ok := apps[r.AppSlug]; !ok || r.AppID < id {
			apps[r.AppSlug] = r.AppID
		}
	}

	owners := make(map[string]string, len(claimants))
	for key, apps := range claimants {
		slugs := make([]string, 0, len(apps))
		for slug := range apps {
			slugs = append(slugs, slug)
		}
		sort.Slice(slugs, func(i, j int) bool {
			if a, b := apps[slugs[i]], apps[slugs[j]]; a != b {
				return a < b
			}
			return slugs[i] < slugs[j]
		})
		owner, ok := prev[key]
		if _, claims := apps[owner]; !ok || !claims {
			owner = slugs[0]
		}
		owners[key] = owner
		for _, slug := range slugs {
			if slug != owner {
				log.Printf("[proxy] WARNING: skip routes for %s of app %q: domain is already served by app %q", key, slug, owner)
			}
		}
	}

	kept := make([]Route, 0, len(routes))
	for _, r := range routes {
		if owners[normalizeDomain(r.Domain)] == r.AppSlug {
			kept = append(kept, r)
		}
	}
	return kept, owners
}

// registerRouteRules replaces the per-domain IP allowlists and rate limiters
// with the ones routes define. claimDomains leaves one app per domain, and an
// app's routes share its config, so the first route carrying one defines it.
func registerRouteRules(routes []Route) {
	limits := map[string]*RateLimitConfig{}
	allow := map[string][]string{}
	for _, r := range routes {
		key := normalizeDomain(r.Domain)
		if _, ok := limits[key]; !ok && r.RateLimit != nil {
			limits[key] = r.RateLimit
		}
		if _, ok := allow[key]; !ok && r.AllowedIPs != nil {
			allow[key] = r.AllowedIPs
		}
	}
	RateLimiters.Replace(limits)
	IPAccessRules.Replace(allow)
}

// routeRules is a copy of the package-level rule registries.
type routeRules struct {
	allow  map[string]*parsedAllowlist
	limits map[string]*domainLimiter
}

func snapshotRouteRules() routeRules {
	return routeRules{allow: IPAccessRules.snapshot(), limits: RateLimiters.snapshot()}
}

// restore puts the copied rules back; limiters keep their counters.
func (s routeRules) restore() {
	IPAccessRules.restore(s.allow)
	RateLimiters.restore(s.limits)
}

// Stop stops all Caddy instances.
func (c *CaddyProxy) Stop() error {
	c.loadMu.Lock()
	c.lastKey = nil
	c.loadMu.Unlock()
	return caddy.Stop()
}

// BuildConfigJSON builds and returns the Caddy JSON config. Exported for testing.
func (c *CaddyProxy) BuildConfigJSON() ([]byte, error) {
	return json.Marshal(c.buildConfigFrom(c.snapshotRoutes()))
}

// snapshotRoutes returns a copy of the current routes.
func (c *CaddyProxy) snapshotRoutes() []Route {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Route(nil), c.routes...)
}

// ForceReload reloads Caddy with the current routes even if the generated
// config is unchanged. Use after files referenced by the config change on
// disk (e.g. a custom cert upload rewrites the same path).
func (c *CaddyProxy) ForceReload() error {
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	return c.reloadLocked(true)
}

// reloadLocked builds the Caddy config and loads it, skipping the load
// (unless force) when the config and the custom cert files it references are
// unchanged. Callers hold loadMu, so concurrent updates cannot load an older
// snapshot after a newer one.
//
// Every caddy.Load is a full reload. On Linux, Caddy binds a fresh
// SO_REUSEPORT socket per reload and closes the old one once the old server
// starts its graceful shutdown; TCP connections the kernel already queued on
// the old socket (accepted by the kernel, not yet by Caddy) are reset, which
// clients see as "connection reset by peer" during the TLS handshake. Avoiding
// no-op reloads (e.g. a redeploy that leaves routes unchanged) removes most of
// that exposure. Hosts on Linux >= 5.14 can also set
// net.ipv4.tcp_migrate_req=1 so the kernel migrates those queued connections
// to the new socket.
func (c *CaddyProxy) reloadLocked(force bool) error {
	// One snapshot for both the config and the cert fingerprint.
	routes := c.snapshotRoutes()
	data, err := json.Marshal(c.buildConfigFrom(routes))
	if err != nil {
		return err
	}
	// The config only holds cert PATHS; fold file metadata into the key so a
	// cert rewritten in place still triggers a reload.
	key := append(append([]byte(nil), data...), certFingerprint(routes)...)
	if !force && c.lastKey != nil && bytes.Equal(c.lastKey, key) {
		return nil
	}
	if err := c.load(data, true); err != nil {
		c.lastKey = nil
		return err
	}
	c.lastKey = key
	return nil
}

// certFingerprint returns size+mtime for every custom cert/key file the
// routes reference, in route order.
func certFingerprint(routes []Route) []byte {
	var b bytes.Buffer
	seen := map[string]bool{}
	for _, r := range orderRoutes(routes) {
		if r.TLS != "custom" || r.CertDir == "" || seen[r.Domain] {
			continue
		}
		seen[r.Domain] = true
		for _, ext := range []string{".crt", ".key"} {
			path := filepath.Join(r.CertDir, r.Domain+ext)
			if fi, err := os.Stat(path); err == nil {
				fmt.Fprintf(&b, "\n%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
			} else {
				fmt.Fprintf(&b, "\n%s|missing", path)
			}
		}
	}
	return b.Bytes()
}

// securityHeader is a response header SimpleDeploy adds to app responses
// that do not set it.
type securityHeader struct{ name, value string }

// defaultSecurityHeaders go on every app route, in this order.
var defaultSecurityHeaders = []securityHeader{
	{"X-Content-Type-Options", "nosniff"},
	{"X-Frame-Options", "SAMEORIGIN"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
}

// hstsHeader is added only on TLS routes; on plain-HTTP routes it would lock
// visitors into HTTPS for hosts that never serve it.
var hstsHeader = securityHeader{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"}

// securityHeaderHandlers returns one Caddy headers handler per safe-default
// security header for r. Each sets its header when the response is written,
// and only if the app's response does not already carry it: a response
// `require` whose header value is null matches only a missing field. So an
// app that sends its own value keeps it. One handler per header because a
// handler's require gates all of its operations at once.
func securityHeaderHandlers(r Route) []interface{} {
	hdrs := defaultSecurityHeaders
	if r.TLS != "off" && r.TLS != "" {
		hdrs = append(hdrs[:len(hdrs):len(hdrs)], hstsHeader)
	}
	out := make([]interface{}, 0, len(hdrs))
	for _, h := range hdrs {
		out = append(out, map[string]interface{}{
			"handler": "headers",
			"response": map[string]interface{}{
				"require": map[string]interface{}{
					"headers": map[string]interface{}{h.name: nil},
				},
				"set": map[string]interface{}{h.name: []string{h.value}},
			},
		})
	}
	return out
}

// buildConfigFrom returns the Caddy config for routes as a map.
func (c *CaddyProxy) buildConfigFrom(routes []Route) map[string]interface{} {
	// Build route entries.
	var caddyRoutes []interface{}
	routes = orderRoutes(routes)

	// Collect custom TLS cert files and per-route local-TLS domains
	var loadFiles []interface{}
	var localTLSDomains []string
	var customTLSDomains []string
	seenCertDomains := map[string]bool{}
	seenCustom := map[string]bool{}

	for _, r := range routes {
		if r.TLS == "local" {
			localTLSDomains = append(localTLSDomains, r.Domain)
		}
		// Bind the access/ratelimit handlers to this route's domain so they
		// apply its rules even when another route domain (e.g. a second
		// wildcard) also matches the request host.
		ruleKey := normalizeDomain(r.Domain)
		handlers := []interface{}{
			map[string]interface{}{"handler": "simpledeploy_ipaccess", "domain": ruleKey},
			map[string]interface{}{"handler": "simpledeploy_ratelimit", "domain": ruleKey},
			map[string]interface{}{"handler": "simpledeploy_metrics"},
		}
		handlers = append(handlers, securityHeaderHandlers(r)...)
		// The server-wide body idle timeout is off (see buildConfigFrom's
		// server block); apply it per route, except to gRPC/h2c streams.
		if !isH2CUpstream(r) {
			handlers = append(handlers, map[string]interface{}{
				"handler":      "timeouts",
				"read_timeout": int64(requestBodyIdleTimeout),
			})
		}
		handlers = append(handlers, reverseProxyHandler(r))
		caddyRoutes = append(caddyRoutes, map[string]interface{}{
			"match":    []interface{}{routeMatcher(r)},
			"handle":   handlers,
			"terminal": true,
		})

		if r.TLS == "custom" && !seenCustom[r.Domain] {
			seenCustom[r.Domain] = true
			customTLSDomains = append(customTLSDomains, r.Domain)
		}
		if r.TLS == "custom" && r.CertDir != "" && !seenCertDomains[r.Domain] {
			seenCertDomains[r.Domain] = true
			crt := filepath.Join(r.CertDir, r.Domain+".crt")
			key := filepath.Join(r.CertDir, r.Domain+".key")
			// Caddy's file loader fails the whole config load on a missing
			// file, which would block every later reload. Skip it; the domain
			// is in skip_certificates, so it gets TLS errors (never an ACME
			// or internal-CA cert) until a cert is uploaded.
			if !fileExists(crt) || !fileExists(key) {
				log.Printf("[proxy] WARNING: custom cert for %s missing (%s, %s); not loading it", r.Domain, crt, key)
			} else {
				loadFiles = append(loadFiles, map[string]interface{}{
					"certificate": crt,
					"key":         key,
					"tags":        []string{r.Domain},
				})
			}
		}
	}

	if caddyRoutes == nil {
		caddyRoutes = []interface{}{}
	}

	server := map[string]interface{}{
		"listen":           []string{c.listenAddr},
		"routes":           caddyRoutes,
		"max_header_bytes": maxRequestHeaderBytes,
		// Caddy's server-wide request body idle timeout (1m by default)
		// would reset quiet gRPC client-streaming and bidi RPCs. Turn it
		// off here; HTTP routes get it back through a timeouts handler.
		"read_idle_timeout": -1,
	}

	needsLocalTLS := len(localTLSDomains) > 0

	if c.tlsMode == "off" && !needsLocalTLS {
		server["automatic_https"] = map[string]interface{}{
			"disable": true,
		}
		// Plain-HTTP listener (TLS terminated in front of SimpleDeploy, or
		// local testing): also accept HTTP/2 cleartext so gRPC clients can
		// connect with prior knowledge. TLS listeners negotiate h2 via ALPN
		// with Caddy's default protocols.
		server["protocols"] = []string{"h1", "h2", "h2c"}
	} else {
		// Caddy only terminates TLS on servers with a connection policy; an
		// empty policy is enough to make the listener serve TLS and let the
		// tls automation app provide certs. Disable implicit HTTP->HTTPS
		// redirects because enabling them tries to bind :80 which is almost
		// never available on test/CI runners and blocks the whole reload.
		// The optional HTTPListenAddr block below covers that case when the
		// user explicitly wants the redirect.
		server["tls_connection_policies"] = []interface{}{map[string]interface{}{}}
		ah := map[string]interface{}{
			"disable_redirects": true,
		}
		// Custom-TLS domains are never managed by automation, even when the
		// cert file is missing; otherwise Caddy would issue an ACME or
		// internal-CA cert for them.
		if len(customTLSDomains) > 0 {
			ah["skip_certificates"] = customTLSDomains
		}
		server["automatic_https"] = ah
	}

	servers := map[string]interface{}{
		"proxy": server,
	}

	// Extra listeners (e.g. :50051 for gRPC) run as a separate server with
	// the same routes and TLS policy but no HTTP/3. If they shared the main
	// server, its Alt-Svc/h3 state would be tied to long-lived h2 streams on
	// the side port: during a reload the old server's QUIC listeners close
	// while those streams keep it alive, and quic-go logs "no port can be
	// announced" for every request. Certs come from the shared tls app cache.
	if len(c.extraListenAddrs) > 0 {
		// Deep copies so later edits to one server never leak into the other.
		extra := map[string]interface{}{
			"listen": append([]string(nil), c.extraListenAddrs...),
			"routes": cloneJSONValue(caddyRoutes),
		}
		for _, k := range []string{"tls_connection_policies", "automatic_https", "max_header_bytes", "read_idle_timeout"} {
			if v, ok := server[k]; ok {
				extra[k] = cloneJSONValue(v)
			}
		}
		if p, ok := server["protocols"]; ok {
			extra["protocols"] = p // TLS off: h1/h2/h2c, already no h3
		} else {
			extra["protocols"] = []string{"h1", "h2"}
		}
		servers["proxy_extra"] = extra
	}

	// Optional HTTP listener that 308-redirects every request to HTTPS. Only
	// meaningful when the main server serves TLS.
	// The TLS branch above already disabled Caddy's implicit :80 redirect so
	// it doesn't race with ours.
	if c.httpListenAddr != "" && c.tlsMode != "off" {
		servers["proxy_http"] = map[string]interface{}{
			"listen":           []string{c.httpListenAddr},
			"max_header_bytes": maxRequestHeaderBytes,
			"routes": []interface{}{
				map[string]interface{}{
					"handle": []interface{}{
						map[string]interface{}{
							"handler":     "static_response",
							"status_code": 308,
							"headers": map[string]interface{}{
								"Location": []string{"https://{http.request.host}{http.request.uri}"},
							},
						},
					},
				},
			},
			"automatic_https": map[string]interface{}{
				"disable": true,
			},
		}
	}

	cfg := map[string]interface{}{
		"admin": map[string]interface{}{
			"disabled": true,
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": servers,
			},
		},
	}

	// Pin Caddy storage under data_dir for all TLS modes. Without this,
	// certmagic falls back to $HOME/.local/share/caddy, which is masked by
	// systemd's ProtectHome=true in the shipped unit and breaks tls.mode=auto.
	if c.dataDir != "" {
		cfg["storage"] = map[string]interface{}{
			"module": "file_system",
			"root":   filepath.Join(c.dataDir, "caddy"),
		}
	}

	// Build TLS config
	tlsCfg := map[string]interface{}{}
	hasTLS := false

	if c.tlsMode == "auto" && c.tlsEmail != "" {
		tlsCfg["automation"] = map[string]interface{}{
			"policies": []interface{}{
				map[string]interface{}{
					"issuers": []interface{}{
						map[string]interface{}{
							"module": "acme",
							"email":  c.tlsEmail,
						},
					},
				},
			},
		}
		hasTLS = true
	}

	if c.tlsMode == "local" {
		tlsCfg["automation"] = map[string]interface{}{
			"policies": []interface{}{
				map[string]interface{}{
					"issuers": []interface{}{
						map[string]interface{}{
							"module": "internal",
						},
					},
				},
			},
		}
		hasTLS = true
	}

	// Per-route local TLS: add subject-scoped internal CA policies when global mode won't cover them.
	if needsLocalTLS && c.tlsMode != "local" {
		existing, _ := tlsCfg["automation"].(map[string]interface{})
		var policies []interface{}
		if existing != nil {
			policies, _ = existing["policies"].([]interface{})
		}
		seen := map[string]bool{}
		for _, domain := range localTLSDomains {
			if seen[domain] {
				continue
			}
			seen[domain] = true
			policies = append(policies, map[string]interface{}{
				"subjects": []string{domain},
				"issuers": []interface{}{
					map[string]interface{}{"module": "internal"},
				},
			})
		}
		if existing == nil {
			tlsCfg["automation"] = map[string]interface{}{"policies": policies}
		} else {
			existing["policies"] = policies
			tlsCfg["automation"] = existing
		}
		hasTLS = true
	}

	if len(loadFiles) > 0 {
		tlsCfg["certificates"] = map[string]interface{}{
			"load_files": loadFiles,
		}
		hasTLS = true
	}

	if hasTLS {
		cfg["apps"].(map[string]interface{})["tls"] = tlsCfg
	}

	return cfg
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// cloneJSONValue deep-copies the map/slice values used to build Caddy config.
func cloneJSONValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = cloneJSONValue(val)
		}
		return out
	case map[string][]string:
		out := make(map[string][]string, len(t))
		for k, val := range t {
			out[k] = append([]string(nil), val...)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = cloneJSONValue(val)
		}
		return out
	case []string:
		return append([]string(nil), t...)
	default:
		return v
	}
}
