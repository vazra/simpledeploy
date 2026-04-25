package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	register("alert", "added", renderAlertAdded)
	register("alert", "removed", renderAlertRemoved)
	register("alert", "changed", renderAlertChanged)
}

type alertView struct {
	Name      string  `json:"name"`
	Metric    string  `json:"metric"`
	Threshold float64 `json:"threshold"`
}

func renderAlertAdded(before, after []byte) (string, string) {
	var a alertView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Alert rule %q added", a.Name), a.Name
}

func renderAlertRemoved(before, after []byte) (string, string) {
	var b alertView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Alert rule %q removed", b.Name), b.Name
}

func renderAlertChanged(before, after []byte) (string, string) {
	var b, a alertView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	name := a.Name
	if name == "" {
		name = b.Name
	}

	var diffs []string
	if b.Metric != a.Metric {
		diffs = append(diffs, fmt.Sprintf("metric %s → %s", b.Metric, a.Metric))
	}
	if b.Threshold != a.Threshold {
		diffs = append(diffs, fmt.Sprintf("threshold %.4g → %.4g", b.Threshold, a.Threshold))
	}

	if len(diffs) == 0 {
		return fmt.Sprintf("Alert rule %q updated", name), name
	}
	return fmt.Sprintf("Alert rule %q: %s", name, strings.Join(diffs, ", ")), name
}
