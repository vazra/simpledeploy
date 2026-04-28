package events

import "fmt"

// Topic constants for the global scopes. Per-app topics are formed via AppTopic.
const (
	TopicGlobalApps       = "global:apps"
	TopicGlobalSettings   = "global:settings"
	TopicGlobalUsers      = "global:users"
	TopicGlobalRegistries = "global:registries"
	TopicGlobalAlerts     = "global:alerts"
	TopicGlobalBackups    = "global:backups"
	TopicGlobalDocker     = "global:docker"
	TopicGlobalAudit      = "global:audit"
)

// AppTopic returns the per-app topic string for a slug.
func AppTopic(slug string) string { return fmt.Sprintf("app:%s", slug) }

// TopicsForAudit returns the list of topics that should fire for a given
// audit category and (optional) app slug. Used by the audit recorder to
// translate mutation events into bus publishes.
func TopicsForAudit(category, slug string) []string {
	out := []string{TopicGlobalAudit}
	switch category {
	case "compose", "endpoint", "env":
		if slug != "" {
			out = append(out, AppTopic(slug))
		}
	case "lifecycle":
		if slug != "" {
			out = append(out, AppTopic(slug))
		}
		out = append(out, TopicGlobalApps)
	case "deploy":
		if slug != "" {
			out = append(out, AppTopic(slug))
		}
	case "backup":
		if slug != "" {
			out = append(out, AppTopic(slug))
		}
		out = append(out, TopicGlobalBackups)
	case "alert":
		if slug != "" {
			out = append(out, AppTopic(slug))
		}
		out = append(out, TopicGlobalAlerts)
	case "webhook":
		out = append(out, TopicGlobalAlerts)
	case "registry":
		out = append(out, TopicGlobalRegistries)
	case "access", "user":
		out = append(out, TopicGlobalUsers)
	case "settings", "gitsync", "audit_config":
		out = append(out, TopicGlobalSettings)
	case "system":
		out = append(out, TopicGlobalSettings)
	case "docker":
		out = append(out, TopicGlobalDocker)
	case "auth":
		// auth events feed the audit log but no other topic
	}
	return out
}

// TypeForCategory returns the canonical event type string for an audit
// category, used as the "type" field in the WS frame.
func TypeForCategory(category string) string {
	switch category {
	case "compose", "endpoint", "env":
		return "app.changed"
	case "lifecycle":
		return "app.status"
	case "deploy":
		return "app.deploy"
	case "backup":
		return "backup.changed"
	case "alert", "webhook":
		return "alert.changed"
	case "registry":
		return "registry.changed"
	case "access", "user":
		return "user.changed"
	case "settings", "gitsync", "audit_config", "system":
		return "settings.changed"
	case "docker":
		return "docker.changed"
	}
	return "audit.appended"
}
