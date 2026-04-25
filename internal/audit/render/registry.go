package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	register("registry", "added", renderRegistryAdded)
	register("registry", "removed", renderRegistryRemoved)
	register("registry", "changed", renderRegistryChanged)
}

type registryView struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// credentials intentionally omitted
}

func renderRegistryAdded(before, after []byte) (string, string) {
	var a registryView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Registry %q added", a.Name), a.Name
}

func renderRegistryRemoved(before, after []byte) (string, string) {
	var b registryView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Registry %q removed", b.Name), b.Name
}

func renderRegistryChanged(before, after []byte) (string, string) {
	var b, a registryView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	name := a.Name
	if name == "" {
		name = b.Name
	}

	var diffs []string
	if b.URL != a.URL {
		diffs = append(diffs, fmt.Sprintf("url %s → %s", b.URL, a.URL))
	}

	if len(diffs) == 0 {
		return fmt.Sprintf("Registry %q updated", name), name
	}
	return fmt.Sprintf("Registry %q: %s", name, strings.Join(diffs, ", ")), name
}
