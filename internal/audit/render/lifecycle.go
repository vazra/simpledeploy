package render

import (
	"encoding/json"
	"fmt"
)

func init() {
	register("lifecycle", "created", renderLifecycleCreated)
	register("lifecycle", "renamed", renderLifecycleRenamed)
	register("lifecycle", "removed", renderLifecycleRemoved)
	register("lifecycle", "stopped", renderLifecycleStopped)
	register("lifecycle", "started", renderLifecycleStarted)
	register("lifecycle", "restarted", renderLifecycleRestarted)
	register("lifecycle", "scaled", renderLifecycleScaled)
}

type lifecycleView struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Replicas int    `json:"replicas"`
}

func renderLifecycleCreated(before, after []byte) (string, string) {
	var a lifecycleView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App %q created", a.Name), a.Name
}

func renderLifecycleRenamed(before, after []byte) (string, string) {
	var b, a lifecycleView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App renamed: %s → %s", b.Name, a.Name), a.Name
}

func renderLifecycleRemoved(before, after []byte) (string, string) {
	var b lifecycleView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("App %q removed", b.Name), b.Name
}

func renderLifecycleStopped(before, after []byte) (string, string) {
	var a lifecycleView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App %q stopped", a.Name), a.Name
}

func renderLifecycleStarted(before, after []byte) (string, string) {
	var a lifecycleView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App %q started", a.Name), a.Name
}

func renderLifecycleRestarted(before, after []byte) (string, string) {
	var a lifecycleView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App %q restarted", a.Name), a.Name
}

func renderLifecycleScaled(before, after []byte) (string, string) {
	var b, a lifecycleView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("App %q scaled: %d → %d", a.Name, b.Replicas, a.Replicas), a.Name
}
