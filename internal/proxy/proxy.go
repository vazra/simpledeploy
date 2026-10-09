package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	caddy "github.com/caddyserver/caddy/v2"
	_ "github.com/caddyserver/caddy/v2/modules/standard"
)

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

// SetRoutes stores routes, configures rate limiters, and reloads Caddy config.
func (c *CaddyProxy) SetRoutes(routes []Route) error {
	for _, r := range routes {
		if !validDomainRe.MatchString(r.Domain) {
			return fmt.Errorf("invalid domain %q", r.Domain)
		}
	}

	c.mu.Lock()
	c.routes = routes
	c.mu.Unlock()

	for _, r := range routes {
		if r.RateLimit != nil {
			RateLimiters.Set(r.Domain, r.RateLimit)
		}
		if r.AllowedIPs != nil {
			IPAccessRules.Set(r.Domain, r.AllowedIPs)
		} else {
			IPAccessRules.Remove(r.Domain)
		}
	}
	return c.reload()
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
	return c.reloadWith(true)
}

// reload builds the Caddy config and loads it, skipping the load when the
// config and the custom cert files it references are unchanged.
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
func (c *CaddyProxy) reload() error {
	return c.reloadWith(false)
}

func (c *CaddyProxy) reloadWith(force bool) error {
	// Build under loadMu so concurrent SetRoutes calls cannot load an older
	// snapshot after a newer one.
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
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

// buildConfigFrom returns the Caddy config for routes as a map.
func (c *CaddyProxy) buildConfigFrom(routes []Route) map[string]interface{} {
	// Build route entries.
	var caddyRoutes []interface{}
	routes = orderRoutes(routes)

	// Collect custom TLS cert files and per-route local-TLS domains
	var loadFiles []interface{}
	var localTLSDomains []string
	seenCertDomains := map[string]bool{}

	for _, r := range routes {
		if r.TLS == "local" {
			localTLSDomains = append(localTLSDomains, r.Domain)
		}
		// Inject safe-default security headers on responses from each app.
		// Defer (rather than overwrite) so an app that already sets these
		// keeps its own value. HSTS is only added when the route uses TLS;
		// adding it on plain-HTTP routes would lock victims into HTTPS for
		// hosts that never serve it.
		headerHandler := map[string]interface{}{
			"handler": "headers",
			"response": map[string]interface{}{
				"deferred": true,
				"set": map[string]interface{}{
					"X-Content-Type-Options": []string{"nosniff"},
					"X-Frame-Options":        []string{"SAMEORIGIN"},
					"Referrer-Policy":        []string{"strict-origin-when-cross-origin"},
				},
			},
		}
		if r.TLS != "off" && r.TLS != "" {
			headerHandler["response"].(map[string]interface{})["set"].(map[string]interface{})["Strict-Transport-Security"] = []string{"max-age=31536000; includeSubDomains"}
		}
		handlers := []interface{}{
			map[string]interface{}{"handler": "simpledeploy_ipaccess"},
			map[string]interface{}{"handler": "simpledeploy_ratelimit"},
			map[string]interface{}{"handler": "simpledeploy_metrics"},
			headerHandler,
			reverseProxyHandler(r),
		}
		caddyRoutes = append(caddyRoutes, map[string]interface{}{
			"match":    []interface{}{routeMatcher(r)},
			"handle":   handlers,
			"terminal": true,
		})

		if r.TLS == "custom" && r.CertDir != "" && !seenCertDomains[r.Domain] {
			seenCertDomains[r.Domain] = true
			crt := filepath.Join(r.CertDir, r.Domain+".crt")
			key := filepath.Join(r.CertDir, r.Domain+".key")
			// Caddy's file loader fails the whole config load on a missing
			// file, which would block every later reload. Skip it; the
			// endpoint falls back to the global TLS automation (or none).
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
		"listen": []string{c.listenAddr},
		"routes": caddyRoutes,
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
		server["automatic_https"] = map[string]interface{}{
			"disable_redirects": true,
		}
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
		for _, k := range []string{"tls_connection_policies", "automatic_https"} {
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
	if c.httpListenAddr != "" && c.tlsMode != "off" {
		// Disable Caddy's implicit :80 redirect so it doesn't race with ours.
		server["automatic_https"] = map[string]interface{}{
			"disable_redirects": true,
		}
		servers["proxy_http"] = map[string]interface{}{
			"listen": []string{c.httpListenAddr},
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
