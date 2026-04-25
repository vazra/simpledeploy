package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func init() {
	register("env", "changed", renderEnvChanged)
}

// envView holds key-only diff data. Values are intentionally not stored in
// the audit log to avoid leaking secrets.
type envView struct {
	Keys []string `json:"keys"`
}

func renderEnvChanged(before, after []byte) (string, string) {
	var b, a envView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	beforeSet := map[string]struct{}{}
	for _, k := range b.Keys {
		beforeSet[k] = struct{}{}
	}
	afterSet := map[string]struct{}{}
	for _, k := range a.Keys {
		afterSet[k] = struct{}{}
	}

	var added, removed []string
	for k := range afterSet {
		if _, ok := beforeSet[k]; !ok {
			added = append(added, k)
		}
	}
	for k := range beforeSet {
		if _, ok := afterSet[k]; !ok {
			removed = append(removed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	var parts []string
	if len(added) > 0 {
		parts = append(parts, fmt.Sprintf("added %s", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		parts = append(parts, fmt.Sprintf("removed %s", strings.Join(removed, ", ")))
	}
	if len(parts) == 0 {
		return "Environment variables updated", ""
	}
	return "Environment variables: " + strings.Join(parts, "; "), ""
}
