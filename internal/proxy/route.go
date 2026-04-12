package proxy

import (
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// Route holds routing config for a deployed app.
type Route struct {
	AppSlug    string
	Domain     string
	Upstream   string // "localhost:{port}"
	TLS        string // "auto", "custom", "off"
	CertDir    string // directory containing certs for custom TLS
	RateLimit  *RateLimitConfig
	AllowedIPs []string // validated IPs and CIDRs
}

// RateLimitConfig holds parsed rate-limit settings for a route.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
	Burst    int
	By       string
}

// ResolveRoutes derives Routes from an AppConfig (one per endpoint).
// Returns empty slice if no endpoints are defined (no error).
func ResolveRoutes(app *compose.AppConfig) ([]Route, error) {
	if len(app.Endpoints) == 0 {
		return nil, nil
	}

	// Parse app-level rate limit and access control once
	var rl *RateLimitConfig
	if app.RateLimit.Requests != "" {
		requests, _ := strconv.Atoi(app.RateLimit.Requests)
		window, _ := time.ParseDuration(app.RateLimit.Window)
		burst, _ := strconv.Atoi(app.RateLimit.Burst)
		by := app.RateLimit.By
		if by == "" {
			by = "ip"
		}
		if requests > 0 {
			rl = &RateLimitConfig{
				Requests: requests,
				Window:   window,
				Burst:    burst,
				By:       by,
			}
		}
	}

	var allowedIPs []string
	if app.AccessAllow != "" {
		for _, entry := range strings.Split(app.AccessAllow, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if net.ParseIP(entry) != nil {
				allowedIPs = append(allowedIPs, entry)
				continue
			}
			if _, _, err := net.ParseCIDR(entry); err == nil {
				allowedIPs = append(allowedIPs, entry)
				continue
			}
			log.Printf("[proxy] ignoring invalid IP/CIDR in access.allow: %q", entry)
		}
	}

	certDir := ""
	if app.ComposePath != "" {
		certDir = filepath.Join(filepath.Dir(app.ComposePath), "certs")
	}

	var routes []Route
	for _, ep := range app.Endpoints {
		if ep.Domain == "" {
			continue
		}

		upstream, err := resolveEndpointUpstream(app, ep)
		if err != nil {
			log.Printf("[proxy] skip endpoint %s for %s: %v", ep.Domain, app.Name, err)
			continue
		}

		tls := ep.TLS
		if tls == "" {
			tls = "auto"
		}

		route := Route{
			AppSlug:    app.Name,
			Domain:     ep.Domain,
			Upstream:   upstream,
			TLS:        tls,
			CertDir:    certDir,
			RateLimit:  rl,
			AllowedIPs: allowedIPs,
		}
		routes = append(routes, route)
	}

	return routes, nil
}

// resolveEndpointUpstream finds the upstream address for an endpoint.
// Looks for a host port mapping matching endpoint.Port in endpoint.Service.
// If no host port mapping exists, uses the Docker network address (service:port).
func resolveEndpointUpstream(app *compose.AppConfig, ep compose.EndpointConfig) (string, error) {
	for _, svc := range app.Services {
		if svc.Name != ep.Service {
			continue
		}
		for _, pm := range svc.Ports {
			if pm.Container == ep.Port {
				if pm.Host != "" {
					return "localhost:" + pm.Host, nil
				}
				// No host mapping, use Docker network
				return fmt.Sprintf("%s:%s", ep.Service, ep.Port), nil
			}
		}
		// Port not in mappings, use Docker network address
		if ep.Port != "" {
			return fmt.Sprintf("%s:%s", ep.Service, ep.Port), nil
		}
		// No port specified, use first mapping
		for _, pm := range svc.Ports {
			if pm.Host != "" {
				return "localhost:" + pm.Host, nil
			}
		}
		return "", fmt.Errorf("service %q has no port mappings", ep.Service)
	}
	// Service not found in parsed services, use Docker network
	if ep.Port != "" {
		return fmt.Sprintf("%s:%s", ep.Service, ep.Port), nil
	}
	return "", fmt.Errorf("service %q not found and no port specified", ep.Service)
}
