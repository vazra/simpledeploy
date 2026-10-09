package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	loadMu     sync.Mutex
	lastLoaded []byte // last config successfully passed to caddy.Load
}

// NewCaddyProxy creates a CaddyProxy from the given config.
func NewCaddyProxy(cfg CaddyConfig) *CaddyProxy {
	return &CaddyProxy{
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
	c.lastLoaded = nil
	c.loadMu.Unlock()
	return caddy.Stop()
}

// BuildConfigJSON builds and returns the Caddy JSON config. Exported for testing.
func (c *CaddyProxy) BuildConfigJSON() ([]byte, error) {
	cfg := c.buildConfig()
	return json.Marshal(cfg)
}

// reload builds the Caddy config and loads it, skipping the load when the
// config is byte-identical to the last one loaded.
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
	// Build under loadMu so concurrent SetRoutes calls cannot load an older
	// snapshot after a newer one.
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	data, err := c.BuildConfigJSON()
	if err != nil {
		return err
	}
	if c.lastLoaded != nil && bytes.Equal(c.lastLoaded, data) {
		return nil
	}
	if err := caddy.Load(data, true); err != nil {
		c.lastLoaded = nil
		return err
	}
	c.lastLoaded = data
	return nil
}

// buildConfig returns the Caddy config as a map.
func (c *CaddyProxy) buildConfig() map[string]interface{} {
	// Build route entries.
	var caddyRoutes []interface{}
	c.mu.Lock()
	routes := make([]Route, len(c.routes))
	copy(routes, c.routes)
	c.mu.Unlock()
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
			loadFiles = append(loadFiles, map[string]interface{}{
				"certificate": filepath.Join(r.CertDir, r.Domain+".crt"),
				"key":         filepath.Join(r.CertDir, r.Domain+".key"),
				"tags":        []string{r.Domain},
			})
		}
	}

	if caddyRoutes == nil {
		caddyRoutes = []interface{}{}
	}

	server := map[string]interface{}{
		"listen": append([]string{c.listenAddr}, c.extraListenAddrs...),
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
		// Extra listeners (e.g. :50051 for gRPC) serve h1/h2 only. HTTP/3
		// stays on the main listener; advertising/serving QUIC on side ports
		// is not useful and Alt-Svc is announced for the main port only.
		if len(c.extraListenAddrs) > 0 {
			lp := make([]interface{}, 0, 1+len(c.extraListenAddrs))
			lp = append(lp, nil) // main listener: server default protocols
			for range c.extraListenAddrs {
				lp = append(lp, []string{"h1", "h2"})
			}
			server["listen_protocols"] = lp
		}
	}

	servers := map[string]interface{}{
		"proxy": server,
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
