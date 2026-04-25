package render

import (
	"encoding/json"
	"fmt"
	"strings"
)

func init() {
	register("backup", "added", renderBackupAdded)
	register("backup", "removed", renderBackupRemoved)
	register("backup", "changed", renderBackupChanged)
}

type backupView struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Target   string `json:"target"`
	Strategy string `json:"strategy"`
}

func renderBackupAdded(before, after []byte) (string, string) {
	var a backupView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Backup %s added (schedule: %s)", a.Name, a.Schedule), a.Name
}

func renderBackupRemoved(before, after []byte) (string, string) {
	var b backupView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Backup %s removed", b.Name), b.Name
}

func renderBackupChanged(before, after []byte) (string, string) {
	var b, a backupView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	name := a.Name
	if name == "" {
		name = b.Name
	}

	var diffs []string
	if b.Schedule != a.Schedule {
		diffs = append(diffs, fmt.Sprintf("schedule %s → %s", b.Schedule, a.Schedule))
	}
	if b.Target != a.Target {
		diffs = append(diffs, fmt.Sprintf("target %s → %s", b.Target, a.Target))
	}
	if b.Strategy != a.Strategy {
		diffs = append(diffs, fmt.Sprintf("strategy %s → %s", b.Strategy, a.Strategy))
	}

	if len(diffs) == 0 {
		return fmt.Sprintf("Backup %s updated", name), name
	}
	return fmt.Sprintf("Backup %s: %s", name, strings.Join(diffs, ", ")), name
}
