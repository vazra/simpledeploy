package render

import (
	"encoding/json"
	"fmt"
)

func init() {
	register("system", "user_added", renderSystemUserAdded)
	register("system", "user_changed", renderSystemUserChanged)
	register("system", "user_removed", renderSystemUserRemoved)
	register("system", "apikey_added", renderSystemApikeyAdded)
	register("system", "apikey_removed", renderSystemApikeyRemoved)
	register("system", "public_host_changed", renderSystemPublicHostChanged)
}

type systemUserView struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type systemApikeyView struct {
	Name     string `json:"name"`
	Username string `json:"username"`
}

type systemHostView struct {
	Host string `json:"host"`
}

func renderSystemUserAdded(before, after []byte) (string, string) {
	var a systemUserView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("User %s added (role: %s)", a.Username, a.Role), a.Username
}

func renderSystemUserChanged(before, after []byte) (string, string) {
	var b, a systemUserView
	_ = json.Unmarshal(before, &b)
	_ = json.Unmarshal(after, &a)
	username := a.Username
	if username == "" {
		username = b.Username
	}
	if b.Role != a.Role {
		return fmt.Sprintf("User %s role: %s → %s", username, b.Role, a.Role), username
	}
	return fmt.Sprintf("User %s updated", username), username
}

func renderSystemUserRemoved(before, after []byte) (string, string) {
	var b systemUserView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("User %s removed", b.Username), b.Username
}

func renderSystemApikeyAdded(before, after []byte) (string, string) {
	var a systemApikeyView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("API key %q added for %s", a.Name, a.Username), a.Username
}

func renderSystemApikeyRemoved(before, after []byte) (string, string) {
	var b systemApikeyView
	_ = json.Unmarshal(before, &b)
	return fmt.Sprintf("API key %q removed", b.Name), b.Username
}

func renderSystemPublicHostChanged(before, after []byte) (string, string) {
	var a systemHostView
	_ = json.Unmarshal(after, &a)
	return fmt.Sprintf("Public host changed to %s", a.Host), ""
}
