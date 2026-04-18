package proxy

import (
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
	ListenAddr     string // e.g. ":443"
	HTTPListenAddr string // optional HTTP listener for plain-HTTP to HTTPS redirect, e.g. ":80"
	TLSMode        string // "auto", "custom", "off", "local"
	TLSEmail       string // ACME email, used when TLSMode is "auto"
	DataDir        string // data directory for Caddy storage
}

// CaddyProxy is a Proxy backed by Caddy.
type CaddyProxy struct {
	mu             sync.Mutex
	routes         []Route
	listenAddr     string
	httpListenAddr string
	tlsMode        string
	tlsEmail       string
	dataDir        string
}

// NewCaddyProxy creates a CaddyProxy from the given config.
func NewCaddyProxy(cfg CaddyConfig) *CaddyProxy {
	return &CaddyProxy{
		listenAddr:     cfg.ListenAddr,
		httpListenAddr: cfg.HTTPListenAddr,
		tlsMode:        cfg.TLSMode,
		tlsEmail:       cfg.TLSEmail,
		dataDir:        cfg.DataDir,
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
	return caddy.Stop()
}

// BuildConfigJSON builds and returns the Caddy JSON config. Exported for testing.
func (c *CaddyProxy) BuildConfigJSON() ([]byte, error) {
	cfg := c.buildConfig()
	return json.Marshal(cfg)
}

// reload builds the Caddy config and loads it.
func (c *CaddyProxy) reload() error {
	data, err := c.BuildConfigJSON()
	if err != nil {
		return err
	}
	return caddy.Load(data, true)
}

// buildConfig returns the Caddy config as a map.
func (c *CaddyProxy) buildConfig() map[string]interface{} {
	// Build route entries.
	var caddyRoutes []interface{}
	c.mu.Lock()
	routes := make([]Route, len(c.routes))
	copy(routes, c.routes)
	c.mu.Unlock()

	// Collect custom TLS cert files
	var loadFiles []interface{}

	for _, r := range routes {
		handlers := []interface{}{
			map[string]interface{}{"handler": "simpledeploy_ipaccess"},
			map[string]interface{}{"handler": "simpledeploy_ratelimit"},
			map[string]interface{}{"handler": "simpledeploy_metrics"},
			map[string]interface{}{
				"handler": "reverse_proxy",
				"upstreams": []interface{}{
					map[string]interface{}{
						"dial": r.Upstream,
					},
				},
			},
		}
		caddyRoutes = append(caddyRoutes, map[string]interface{}{
			"match": []interface{}{
				map[string]interface{}{
					"host": []string{r.Domain},
				},
			},
			"handle": handlers,
		})

		if r.TLS == "custom" && r.CertDir != "" {
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
		"listen": []string{c.listenAddr},
		"routes": caddyRoutes,
	}

	if c.tlsMode == "off" {
		server["automatic_https"] = map[string]interface{}{
			"disable": true,
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

	if c.tlsMode == "local" && c.dataDir != "" {
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
