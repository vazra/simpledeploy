package proxy

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

// upstreamHost returns the host portion to use for localhost-bound upstreams.
// When SIMPLEDEPLOY_UPSTREAM_HOST is set, it overrides "localhost". This lets
// SimpleDeploy run inside a Docker container (e.g. Docker Desktop) where
// "localhost" does not reach host-published ports; set it to
// "host.docker.internal" in that case. Empty (default) preserves native
// behavior.
func upstreamHost() string {
	if h := os.Getenv("SIMPLEDEPLOY_UPSTREAM_HOST"); h != "" {
		return h
	}
	return "localhost"
}

// UpstreamResolver resolves a compose service+port to a concrete upstream
// host string. Implementations may use Docker API lookups to map a service
// name to a container IP on the shared network. A nil resolver disables
// container-IP resolution and callers fall through to DNS.
type UpstreamResolver interface {
	ContainerIP(project, service, network string) (string, error)
}

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
// If resolver is non-nil, endpoints without a published host port try to
// resolve to <container-ip>:<port> via Docker API and fall back to DNS on
// failure or empty result.
func ResolveRoutes(app *compose.AppConfig, resolver UpstreamResolver) ([]Route, error) {
	if len(app.Endpoints) == 0 {
		return nil, nil
	}

	// Parse app-level rate limit and access control once
	var rl *RateLimitConfig
	if app.RateLimit.Requests != "" {
		requests, err := strconv.Atoi(app.RateLimit.Requests)
		if err != nil {
			log.Printf("[proxy] invalid ratelimit.requests %q for %s: %v", app.RateLimit.Requests, app.Name, err)
		}
		window, err := time.ParseDuration(app.RateLimit.Window)
		if err != nil {
			log.Printf("[proxy] invalid ratelimit.window %q for %s: %v, defaulting to 1m", app.RateLimit.Window, app.Name, err)
			window = time.Minute
		}
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

		upstream, err := resolveEndpointUpstream(app, ep, resolver)
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
// If no host port mapping exists, tries container-IP resolution via resolver
// (when non-nil) and falls back to the Docker DNS string (<service>:<port>).
func resolveEndpointUpstream(app *compose.AppConfig, ep compose.EndpointConfig, resolver UpstreamResolver) (string, error) {
	for _, svc := range app.Services {
		if svc.Name != ep.Service {
			continue
		}
		for _, pm := range svc.Ports {
			if pm.Container == ep.Port {
				if pm.Host != "" {
					return upstreamHost() + ":" + pm.Host, nil
				}
				// No host mapping, use container-IP resolution then DNS.
				return containerOrDNS(app, ep, resolver), nil
			}
		}
		// Port not in mappings, use container-IP resolution then DNS.
		if ep.Port != "" {
			return containerOrDNS(app, ep, resolver), nil
		}
		// No port specified, use first mapping
		for _, pm := range svc.Ports {
			if pm.Host != "" {
				return upstreamHost() + ":" + pm.Host, nil
			}
		}
		return "", fmt.Errorf("service %q has no port mappings", ep.Service)
	}
	// Service not found in parsed services, use container-IP resolution then DNS.
	if ep.Port != "" {
		return containerOrDNS(app, ep, resolver), nil
	}
	return "", fmt.Errorf("service %q not found and no port specified", ep.Service)
}

// containerOrDNS attempts a Docker API container-IP lookup; if the resolver
// returns a non-empty IP, the upstream is <ip>:<port> (bypassing the localhost
// rewrite since IPs are already routable). Otherwise falls back to the Docker
// DNS string <service>:<port>.
func containerOrDNS(app *compose.AppConfig, ep compose.EndpointConfig, resolver UpstreamResolver) string {
	if resolver != nil {
		if ip, err := resolver.ContainerIP("simpledeploy-"+app.Name, ep.Service, "simpledeploy-public"); err == nil && ip != "" {
			return ip + ":" + ep.Port
		}
	}
	return fmt.Sprintf("%s:%s", ep.Service, ep.Port)
}
