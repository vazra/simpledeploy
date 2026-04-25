package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	register("endpoint", "added", renderEndpointAdded)
	register("endpoint", "removed", renderEndpointRemoved)
	register("endpoint", "changed", renderEndpointChanged)
}

type endpointView struct {
	Host string `json:"host"`
	TLS  bool   `json:"tls"`
	Path string `json:"path"`
}

func renderEndpointAdded(before, after []byte) (string, string) {
	var a endpointView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Endpoint %s added", a.Host), a.Host
}

func renderEndpointRemoved(before, after []byte) (string, string) {
	var b endpointView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Endpoint %s removed", b.Host), b.Host
}

func renderEndpointChanged(before, after []byte) (string, string) {
	var b, a endpointView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	var diffs []string
	if !b.TLS && a.TLS {
		diffs = append(diffs, "TLS enabled")
	} else if b.TLS && !a.TLS {
		diffs = append(diffs, "TLS disabled")
	}
	if b.Path != a.Path {
		diffs = append(diffs, fmt.Sprintf("path %s → %s", b.Path, a.Path))
	}

	host := a.Host
	if host == "" {
		host = b.Host
	}
	if len(diffs) == 0 {
		return fmt.Sprintf("Endpoint %s updated", host), host
	}
	return fmt.Sprintf("Endpoint %s: %s", host, strings.Join(diffs, ", ")), host
}
