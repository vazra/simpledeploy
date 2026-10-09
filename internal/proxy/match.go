package proxy

import (
	"sort"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
)

// isH2CUpstream reports whether the route proxies with HTTP/2 cleartext.
func isH2CUpstream(r Route) bool {
	return r.Protocol == compose.ProtocolGRPC || r.Protocol == compose.ProtocolH2C
}

// routeRank orders routes sharing a domain: grpc (most specific matcher)
// first, then path routes, then the catch-all.
func routeRank(r Route) int {
	switch {
	case r.Protocol == compose.ProtocolGRPC:
		return 0
	case r.Path != "":
		return 1
	default:
		return 2
	}
}

// orderRoutes returns a new slice with routes grouped by domain and, inside
// each domain, sorted by routeRank and then longest path first. Caddy
// evaluates routes top to bottom and every route is terminal, so the most
// specific matcher must come first. Domains are sorted (exact hosts before
// wildcards, then lexically) and equal-length paths lexically; exact duplicates
// keep input order, which callers make deterministic (apps sorted by slug,
// endpoints by label index). A stable config lets SetRoutes skip no-op reloads.
func orderRoutes(in []Route) []Route {
	groups := map[string][]Route{}
	var domains []string
	for _, r := range in {
		if _, ok := groups[r.Domain]; !ok {
			domains = append(domains, r.Domain)
		}
		groups[r.Domain] = append(groups[r.Domain], r)
	}
	out := make([]Route, 0, len(in))
	sort.Slice(domains, func(i, j int) bool {
		wi, wj := strings.Contains(domains[i], "*"), strings.Contains(domains[j], "*")
		if wi != wj {
			return !wi
		}
		return domains[i] < domains[j]
	})
	for _, d := range domains {
		g := groups[d]
		sort.SliceStable(g, func(i, j int) bool {
			ri, rj := routeRank(g[i]), routeRank(g[j])
			if ri != rj {
				return ri < rj
			}
			if len(g[i].Path) != len(g[j].Path) {
				return len(g[i].Path) > len(g[j].Path)
			}
			// Same domain+path: stable sort keeps first-seen (label/app order).
			return g[i].Path < g[j].Path
		})
		out = append(out, g...)
	}
	return out
}

// routeMatcher builds the Caddy matcher set for a route.
func routeMatcher(r Route) map[string]interface{} {
	m := map[string]interface{}{
		"host": []string{r.Domain},
	}
	if r.Path != "" {
		m["path"] = []string{r.Path}
	}
	if r.Protocol == compose.ProtocolGRPC {
		m["header"] = map[string][]string{"Content-Type": {"application/grpc*"}}
		// gRPC-Web (application/grpc-web, application/grpc-web+proto,
		// application/grpc-web-text) also matches the prefix above but is
		// served by the HTTP/1.1 upstream, so exclude it explicitly.
		m["not"] = []interface{}{
			map[string]interface{}{"header": map[string][]string{"Content-Type": {"application/grpc-web*"}}},
		}
	}
	return m
}

// reverseProxyHandler builds the reverse_proxy handler for a route. h2c and
// grpc routes talk HTTP/2 cleartext to the upstream and flush every write
// so server-streaming and bidi RPCs are not buffered.
func reverseProxyHandler(r Route) map[string]interface{} {
	h := map[string]interface{}{
		"handler": "reverse_proxy",
		"upstreams": []interface{}{
			map[string]interface{}{
				"dial": r.Upstream,
			},
		},
	}
	if isH2CUpstream(r) {
		h["transport"] = map[string]interface{}{
			"protocol": "http",
			"versions": []string{"h2c"},
		}
		h["flush_interval"] = -1
	}
	return h
}
