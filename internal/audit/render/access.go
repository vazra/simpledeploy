package render

import (
	"encoding/json"
	"fmt"
)

func init() {
	register("access", "added", renderAccessAdded)
	register("access", "removed", renderAccessRemoved)
	register("access", "changed", renderAccessChanged)
}

type accessView struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

func renderAccessAdded(before, after []byte) (string, string) {
	var a accessView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Access granted: %s (%s)", a.Username, a.Role), a.Username
}

func renderAccessRemoved(before, after []byte) (string, string) {
	var b accessView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("Access revoked: %s", b.Username), b.Username
}

func renderAccessChanged(before, after []byte) (string, string) {
	var b, a accessView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	username := a.Username
	if username == "" {
		username = b.Username
	}
	return fmt.Sprintf("Access role for %s: %s → %s", username, b.Role, a.Role), username
}
