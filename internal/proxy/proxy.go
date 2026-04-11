package proxy

import (
	"encoding/json"
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
	ListenAddr string // e.g. ":443"
	TLSMode    string // "auto", "custom", "off"
	TLSEmail   string // ACME email, used when TLSMode is "auto"
}

// CaddyProxy is a Proxy backed by Caddy.
type CaddyProxy struct {
	mu         sync.Mutex
	routes     []Route
	listenAddr string
	tlsMode    string
	tlsEmail   string
}

// NewCaddyProxy creates a CaddyProxy from the given config.
func NewCaddyProxy(cfg CaddyConfig) *CaddyProxy {
	return &CaddyProxy{
		listenAddr: cfg.ListenAddr,
		tlsMode:    cfg.TLSMode,
		tlsEmail:   cfg.TLSEmail,
	}
}

// SetRoutes stores routes, configures rate limiters, and reloads Caddy config.
func (c *CaddyProxy) SetRoutes(routes []Route) error {
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

	cfg := map[string]interface{}{
		"admin": map[string]interface{}{
			"disabled": true,
		},
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"proxy": server,
				},
			},
		},
	}

	if c.tlsMode == "auto" && c.tlsEmail != "" {
		cfg["apps"].(map[string]interface{})["tls"] = map[string]interface{}{
			"automation": map[string]interface{}{
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
			},
		}
	}

	return cfg
}
