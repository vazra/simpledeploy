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
	register("lifecycle", "image_pulled", renderLifecycleImagePulled)
	register("lifecycle", "archived", renderLifecycleArchived)
	register("lifecycle", "purged", renderLifecyclePurged)
	register("lifecycle", "exported", renderLifecycleExported)
	register("lifecycle", "imported", renderLifecycleImported)
}

func renderLifecycleExported(before, after []byte) (string, string) {
	var v lifecycleView
	if len(after) > 0 {
		_ = json.Unmarshal(after, &v)
	}
	if v.Name == "" {
		_ = json.Unmarshal(before, &v)
	}
	return fmt.Sprintf("App %q exported as bundle", v.Name), v.Name
}

func renderLifecycleImported(before, after []byte) (string, string) {
	var v lifecycleView
	if len(after) > 0 {
		_ = json.Unmarshal(after, &v)
	}
	if v.Name == "" {
		_ = json.Unmarshal(before, &v)
	}
	if v.Mode != "" {
		return fmt.Sprintf("App %q imported from bundle (%s)", v.Name, v.Mode), v.Name
	}
	return fmt.Sprintf("App %q imported from bundle", v.Name), v.Name
}

func renderLifecycleArchived(before, after []byte) (string, string) {
	var v lifecycleView
	if len(after) > 0 {
		_ = json.Unmarshal(after, &v)
	}
	if v.Name == "" {
		_ = json.Unmarshal(before, &v)
	}
	return fmt.Sprintf("App %q archived (directory removed from disk)", v.Name), v.Name
}

func renderLifecyclePurged(before, after []byte) (string, string) {
	var v lifecycleView
	if len(after) > 0 {
		_ = json.Unmarshal(after, &v)
	}
	if v.Name == "" {
		_ = json.Unmarshal(before, &v)
	}
	if v.RowsDeleted > 0 {
		return fmt.Sprintf("App %q purged (%d history rows deleted)", v.Name, v.RowsDeleted), v.Name
	}
	return fmt.Sprintf("App %q purged", v.Name), v.Name
}

func renderLifecycleImagePulled(before, after []byte) (string, string) {
	var a lifecycleView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Images pulled for %q", a.Name), a.Name
}

type lifecycleView struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Replicas    int    `json:"replicas"`
	RowsDeleted int    `json:"rows_deleted"`
	Mode        string `json:"mode"`
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
