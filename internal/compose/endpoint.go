package compose

import "strings"

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

// NormalizeProtocol lowercases and trims a protocol label value.
func NormalizeProtocol(p string) string {
	return strings.ToLower(strings.TrimSpace(p))
}
