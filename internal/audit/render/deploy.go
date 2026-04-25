package render

import (
	"encoding/json"
	"fmt"
)

func init() {
	register("deploy", "deploy_succeeded", renderDeploySucceeded)
	register("deploy", "deploy_failed", renderDeployFailed)
	register("deploy", "rollback", renderDeployRollback)
}

type deployView struct {
	Version int    `json:"version"`
	Error   string `json:"error"`
}

func renderDeploySucceeded(before, after []byte) (string, string) {
	var a deployView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Deploy succeeded (version %d)", a.Version), ""
}

func renderDeployFailed(before, after []byte) (string, string) {
	var a deployView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Deploy failed: %s", a.Error), ""
}

func renderDeployRollback(before, after []byte) (string, string) {
	var a deployView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Rolled back to version %d", a.Version), ""
}
