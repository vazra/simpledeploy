package compose

import (
	"fmt"
	"regexp"
	"strings"
)

// Endpoint protocol values for simpledeploy.endpoints.N.protocol.
const (
	// ProtocolHTTP proxies with Caddy's default upstream transport
	// (HTTP/1.1 to plaintext upstreams). This is the default.
	ProtocolHTTP = "http"
	// ProtocolH2C proxies with HTTP/2 cleartext (prior knowledge) to the upstream.
	ProtocolH2C = "h2c"
	// ProtocolGRPC is h2c to the upstream, matched only for native gRPC
	// requests (Content-Type application/grpc*, excluding gRPC-Web).
	ProtocolGRPC = "grpc"
)

// maxEndpointPathLen bounds simpledeploy.endpoints.N.path.
const maxEndpointPathLen = 256

// endpointPathRe allows Caddy path-matcher patterns made of unreserved URL
// path characters, percent escapes, "/" and the "*" wildcard. Must start
// with "/". Query strings, spaces and quotes are rejected.
var endpointPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~%/*-]*$`)

// NormalizeProtocol lowercases and trims a protocol label value.
func NormalizeProtocol(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}

// ValidProtocol reports whether p (already normalized) is a supported
// endpoint protocol. Empty means the default (http).
func ValidProtocol(p string) bool {
	switch p {
	case "", ProtocolHTTP, ProtocolH2C, ProtocolGRPC:
		return true
	}
	return false
}

// ValidPath reports whether p is an acceptable endpoint path matcher.
func ValidPath(p string) bool {
	return len(p) <= maxEndpointPathLen && endpointPathRe.MatchString(p)
}

// EndpointMatchKey identifies the Caddy matcher an endpoint produces.
// Two endpoints with the same key would shadow each other. h2c and http
// differ only in upstream transport, so they share a key; grpc adds a
// Content-Type matcher, so it does not.
func EndpointMatchKey(ep EndpointConfig) string {
	return fmt.Sprintf("%s|grpc=%t|%s", ep.Domain, ep.Protocol == ProtocolGRPC, ep.Path)
}

// ValidateEndpoints checks protocol/path label values and rejects endpoints
// whose matchers collide. Returns human-readable violations; empty = valid.
func ValidateEndpoints(eps []EndpointConfig) []string {
	var violations []string
	seen := map[string]int{}
	for i, ep := range eps {
		if !ValidProtocol(ep.Protocol) {
			violations = append(violations, fmt.Sprintf("endpoint %d (%s): invalid protocol %q (want http, h2c or grpc)", i, ep.Domain, ep.Protocol))
			continue
		}
		if ep.Path != "" && !ValidPath(ep.Path) {
			violations = append(violations, fmt.Sprintf("endpoint %d (%s): invalid path %q (must start with / and use only letters, digits, . _ ~ %% / * -; max %d chars)", i, ep.Domain, ep.Path, maxEndpointPathLen))
			continue
		}
		key := EndpointMatchKey(ep)
		if j, dup := seen[key]; dup {
			violations = append(violations, fmt.Sprintf("endpoint %d (%s): duplicates endpoint %d (same domain, path and grpc matcher)", i, ep.Domain, j))
			continue
		}
		seen[key] = i
	}
	return violations
}
