package proxy

import (
	"fmt"
	"log"
	"net"
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

// ResolveRoute derives a Route from an AppConfig.
// Returns an error if app.Domain is empty or no port mapping is found.
func ResolveRoute(app *compose.AppConfig) (*Route, error) {
	if app.Domain == "" {
		return nil, fmt.Errorf("app %q has no domain configured", app.Name)
	}

	hostPort, err := resolveHostPort(app)
	if err != nil {
		return nil, err
	}

	tls := app.TLS
	if tls == "" {
		tls = "auto"
	}

	route := &Route{
		AppSlug:  app.Name,
		Domain:   app.Domain,
		Upstream: "localhost:" + hostPort,
		TLS:      tls,
	}

	if app.RateLimit.Requests != "" {
		requests, _ := strconv.Atoi(app.RateLimit.Requests)
		window, _ := time.ParseDuration(app.RateLimit.Window)
		burst, _ := strconv.Atoi(app.RateLimit.Burst)
		by := app.RateLimit.By
		if by == "" {
			by = "ip"
		}
		if requests > 0 {
			route.RateLimit = &RateLimitConfig{
				Requests: requests,
				Window:   window,
				Burst:    burst,
				By:       by,
			}
		}
	}

	if app.AccessAllow != "" {
		for _, entry := range strings.Split(app.AccessAllow, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if net.ParseIP(entry) != nil {
				route.AllowedIPs = append(route.AllowedIPs, entry)
				continue
			}
			if _, _, err := net.ParseCIDR(entry); err == nil {
				route.AllowedIPs = append(route.AllowedIPs, entry)
				continue
			}
			log.Printf("[proxy] ignoring invalid IP/CIDR in access.allow: %q", entry)
		}
	}

	return route, nil
}

// resolveHostPort finds the host port from app.Services port mappings.
// If app.Port is set, it matches by container port; otherwise uses the first mapping.
func resolveHostPort(app *compose.AppConfig) (string, error) {
	for _, svc := range app.Services {
		for _, pm := range svc.Ports {
			if app.Port == "" {
				if pm.Host == "" {
					continue
				}
				return pm.Host, nil
			}
			if pm.Container == app.Port {
				if pm.Host == "" {
					return "", fmt.Errorf("app %q: container port %s has no host port mapping", app.Name, app.Port)
				}
				return pm.Host, nil
			}
		}
	}

	if app.Port != "" {
		return "", fmt.Errorf("app %q: no port mapping found for container port %s", app.Name, app.Port)
	}
	return "", fmt.Errorf("app %q: no port mappings found", app.Name)
}
