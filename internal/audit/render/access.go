package render

import (
	"encoding/json"
	"fmt"
)

func init() {
	register("access", "added", renderAccessAdded)
	register("access", "removed", renderAccessRemoved)
	register("access", "changed", renderAccessChanged)
	register("access", "iplist_changed", renderAccessIPListChanged)
}

type ipListView struct {
	Allow []string `json:"allow"`
}

func renderAccessIPListChanged(before, after []byte) (string, string) {
	var b, a ipListView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)

	beforeSet := map[string]struct{}{}
	for _, e := range b.Allow {
		beforeSet[e] = struct{}{}
	}
	afterSet := map[string]struct{}{}
	for _, e := range a.Allow {
		afterSet[e] = struct{}{}
	}
	added, removed := 0, 0
	for k := range afterSet {
		if _, ok := beforeSet[k]; !ok {
			added++
		}
	}
	for k := range beforeSet {
		if _, ok := afterSet[k]; !ok {
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return "IP allowlist updated", ""
	}
	return fmt.Sprintf("IP allowlist: %d entries added, %d removed", added, removed), ""
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
