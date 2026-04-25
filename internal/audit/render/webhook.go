package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	register("webhook", "added", renderWebhookAdded)
	register("webhook", "removed", renderWebhookRemoved)
	register("webhook", "changed", renderWebhookChanged)
}

type webhookView struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

func (w webhookView) label() string {
	if w.Name != "" {
		return w.Name
	}
	return w.URL
}

func renderWebhookAdded(before, after []byte) (string, string) {
	var a webhookView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Webhook %q added", a.label()), a.label()
}

func renderWebhookRemoved(before, after []byte) (string, string) {
	var b webhookView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Webhook %q removed", b.label()), b.label()
}

func renderWebhookChanged(before, after []byte) (string, string) {
	var b, a webhookView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	lbl := a.label()
	if lbl == "" {
		lbl = b.label()
	}

	var diffs []string
	if b.URL != a.URL {
		diffs = append(diffs, fmt.Sprintf("url %s → %s", b.URL, a.URL))
	}
	bEvents := strings.Join(b.Events, ",")
	aEvents := strings.Join(a.Events, ",")
	if bEvents != aEvents {
		diffs = append(diffs, fmt.Sprintf("events [%s] → [%s]", bEvents, aEvents))
	}

	if len(diffs) == 0 {
		return fmt.Sprintf("Webhook %q updated", lbl), lbl
	}
	return fmt.Sprintf("Webhook %q: %s", lbl, strings.Join(diffs, ", ")), lbl
}
