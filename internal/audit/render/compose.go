package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func init() {
	register("compose", "changed", renderComposeChanged)
}

type composeView struct {
	Services map[string]composeService `json:"services"`
}
type composeService struct {
	Image    string            `json:"image"`
	Env      map[string]string `json:"env"`
	Ports    []string          `json:"ports"`
	Replicas int               `json:"replicas"`
	Labels   map[string]string `json:"labels"`
}

func renderComposeChanged(before, after []byte) (string, string) {
	var b, a composeView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	var diffs []string
	var firstSvc string
	names := unionKeys(b.Services, a.Services)
	sort.Strings(names)
	for _, name := range names {
		bs, hasB := b.Services[name]
		as, hasA := a.Services[name]
		switch {
		case !hasB && hasA:
			diffs = append(diffs, fmt.Sprintf("service %s added", name))
		case hasB && !hasA:
			diffs = append(diffs, fmt.Sprintf("service %s removed", name))
		default:
			diffs = append(diffs, svcDiffs(name, bs, as)...)
		}
		// firstSvc takes the first service in sorted order; cheap and stable for a single target hint.
		if firstSvc == "" {
			firstSvc = name
		}
	}
	if len(diffs) == 0 {
		return "Compose updated (no field-level changes)", firstSvc
	}
	return "Compose changed: " + strings.Join(diffs, "; "), firstSvc
}

func svcDiffs(name string, b, a composeService) []string {
	var out []string
	if b.Image != a.Image {
		out = append(out, fmt.Sprintf("%s image %s → %s", name, b.Image, a.Image))
	}
	if b.Replicas != a.Replicas {
		out = append(out, fmt.Sprintf("%s replicas %d → %d", name, b.Replicas, a.Replicas))
	}
	for _, k := range unionKeys(b.Env, a.Env) {
		bv, hasB := b.Env[k]
		av, hasA := a.Env[k]
		switch {
		case !hasB && hasA:
			out = append(out, fmt.Sprintf("%s env %s added", name, k))
		case hasB && !hasA:
			out = append(out, fmt.Sprintf("%s env %s removed", name, k))
		case bv != av:
			out = append(out, fmt.Sprintf("%s env %s changed", name, k))
		}
	}
	return out
}

func unionKeys[V any](a, b map[string]V) []string {
	seen := map[string]struct{}{}
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
