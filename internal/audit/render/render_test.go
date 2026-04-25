package render

import (
	"strings"
	"testing"
)

func TestUnknownFallback(t *testing.T) {
	s, target := Render("totally", "unknown", nil, nil)
	if s != "totally: unknown" {
		t.Errorf("unexpected fallback: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

func TestComposeImageChange(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	after := []byte(`{"services":{"web":{"image":"nginx:1.26"}}}`)
	s, target := Render("compose", "changed", before, after)
	if !strings.Contains(s, "nginx:1.25") || !strings.Contains(s, "nginx:1.26") {
		t.Errorf("expected before/after images in summary, got %q", s)
	}
	if target != "web" {
		t.Errorf("expected target=web, got %q", target)
	}
}

func TestComposeServiceAddedRemoved(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	after := []byte(`{"services":{"web":{"image":"nginx:1.25"},"redis":{"image":"redis:7"}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "redis added") {
		t.Errorf("expected redis added in summary, got %q", s)
	}

	before2 := []byte(`{"services":{"web":{"image":"nginx:1.25"},"old":{"image":"x"}}}`)
	after2 := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	s2, _ := Render("compose", "changed", before2, after2)
	if !strings.Contains(s2, "old removed") {
		t.Errorf("expected old removed in summary, got %q", s2)
	}
}

func TestComposeEnvDiff(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"2"}}}}`)
	after := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"3","C":"4"}}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "env B changed") {
		t.Errorf("missing env B changed: %q", s)
	}
	if !strings.Contains(s, "env C added") {
		t.Errorf("missing env C added: %q", s)
	}
}

func TestComposeReplicasChange(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","replicas":1}}}`)
	after := []byte(`{"services":{"web":{"image":"x","replicas":3}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "replicas 1 → 3") {
		t.Errorf("expected replicas diff in summary, got %q", s)
	}
}

func TestComposeEnvRemoved(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"2"}}}}`)
	after := []byte(`{"services":{"web":{"image":"x","env":{"A":"1"}}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "env B removed") {
		t.Errorf("expected env B removed in summary, got %q", s)
	}
}

// --- endpoint ---

func TestEndpointAdded(t *testing.T) {
	s, target := Render("endpoint", "added", nil, []byte(`{"host":"a.example.com","tls":false}`))
	if !strings.Contains(s, "a.example.com") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "a.example.com" {
		t.Errorf("target=%q", target)
	}
}

func TestEndpointRemoved(t *testing.T) {
	s, target := Render("endpoint", "removed", []byte(`{"host":"b.example.com"}`), nil)
	if !strings.Contains(s, "b.example.com") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "b.example.com" {
		t.Errorf("target=%q", target)
	}
}

func TestEndpointTLSToggle(t *testing.T) {
	s, _ := Render("endpoint", "changed",
		[]byte(`{"host":"a.example.com","tls":false}`),
		[]byte(`{"host":"a.example.com","tls":true}`))
	if !strings.Contains(s, "TLS enabled") {
		t.Errorf("expected TLS enabled, got %q", s)
	}
}

func TestEndpointTLSDisabled(t *testing.T) {
	s, _ := Render("endpoint", "changed",
		[]byte(`{"host":"a.example.com","tls":true}`),
		[]byte(`{"host":"a.example.com","tls":false}`))
	if !strings.Contains(s, "TLS disabled") {
		t.Errorf("expected TLS disabled, got %q", s)
	}
}

func TestEndpointPathChanged(t *testing.T) {
	s, _ := Render("endpoint", "changed",
		[]byte(`{"host":"a.example.com","path":"/old"}`),
		[]byte(`{"host":"a.example.com","path":"/new"}`))
	if !strings.Contains(s, "/old") || !strings.Contains(s, "/new") {
		t.Errorf("expected path diff, got %q", s)
	}
}

func TestEndpointChangedFallback(t *testing.T) {
	s, target := Render("endpoint", "changed",
		[]byte(`{"host":"x.com","tls":false,"path":"/a"}`),
		[]byte(`{"host":"x.com","tls":false,"path":"/a"}`))
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated, got %q", s)
	}
	if target != "x.com" {
		t.Errorf("target=%q", target)
	}
}

// --- backup ---

func TestBackupAdded(t *testing.T) {
	s, target := Render("backup", "added", nil, []byte(`{"name":"daily","schedule":"0 2 * * *","target":"s3","strategy":"postgres"}`))
	if !strings.Contains(s, "daily") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if !strings.Contains(s, "0 2 * * *") {
		t.Errorf("expected schedule in summary: %q", s)
	}
	if target != "daily" {
		t.Errorf("target=%q", target)
	}
}

func TestBackupRemoved(t *testing.T) {
	s, target := Render("backup", "removed", []byte(`{"name":"weekly"}`), nil)
	if !strings.Contains(s, "weekly") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "weekly" {
		t.Errorf("target=%q", target)
	}
}

func TestBackupChanged(t *testing.T) {
	s, _ := Render("backup", "changed",
		[]byte(`{"name":"daily","schedule":"0 2 * * *","target":"s3","strategy":"postgres"}`),
		[]byte(`{"name":"daily","schedule":"0 3 * * *","target":"s3","strategy":"postgres"}`))
	if !strings.Contains(s, "schedule") || !strings.Contains(s, "0 2 * * *") {
		t.Errorf("expected schedule diff, got %q", s)
	}
}

func TestBackupChangedFallback(t *testing.T) {
	s, _ := Render("backup", "changed",
		[]byte(`{"name":"daily","schedule":"0 2 * * *"}`),
		[]byte(`{"name":"daily","schedule":"0 2 * * *"}`))
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated, got %q", s)
	}
}

// --- alert ---

func TestAlertAdded(t *testing.T) {
	s, target := Render("alert", "added", nil, []byte(`{"name":"high-cpu","metric":"cpu","threshold":90}`))
	if !strings.Contains(s, "high-cpu") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "high-cpu" {
		t.Errorf("target=%q", target)
	}
}

func TestAlertRemoved(t *testing.T) {
	s, target := Render("alert", "removed", []byte(`{"name":"high-cpu"}`), nil)
	if !strings.Contains(s, "high-cpu") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "high-cpu" {
		t.Errorf("target=%q", target)
	}
}

func TestAlertChanged(t *testing.T) {
	s, _ := Render("alert", "changed",
		[]byte(`{"name":"high-cpu","metric":"cpu","threshold":90}`),
		[]byte(`{"name":"high-cpu","metric":"cpu","threshold":95}`))
	if !strings.Contains(s, "threshold") {
		t.Errorf("expected threshold diff, got %q", s)
	}
}

func TestAlertChangedMetric(t *testing.T) {
	s, _ := Render("alert", "changed",
		[]byte(`{"name":"r","metric":"cpu","threshold":90}`),
		[]byte(`{"name":"r","metric":"memory","threshold":90}`))
	if !strings.Contains(s, "metric") || !strings.Contains(s, "memory") {
		t.Errorf("expected metric diff, got %q", s)
	}
}

func TestAlertChangedFallback(t *testing.T) {
	s, _ := Render("alert", "changed",
		[]byte(`{"name":"r","metric":"cpu","threshold":90}`),
		[]byte(`{"name":"r","metric":"cpu","threshold":90}`))
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated, got %q", s)
	}
}

// --- webhook ---

func TestWebhookAdded(t *testing.T) {
	s, target := Render("webhook", "added", nil, []byte(`{"name":"my-hook","url":"https://hooks.example.com/1","events":["deploy"]}`))
	if !strings.Contains(s, "my-hook") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "my-hook" {
		t.Errorf("target=%q", target)
	}
}

func TestWebhookAddedNoName(t *testing.T) {
	s, target := Render("webhook", "added", nil, []byte(`{"url":"https://hooks.example.com/2","events":[]}`))
	if !strings.Contains(s, "https://hooks.example.com/2") {
		t.Errorf("expected url as label, got %q", s)
	}
	if target != "https://hooks.example.com/2" {
		t.Errorf("target=%q", target)
	}
}

func TestWebhookRemoved(t *testing.T) {
	s, target := Render("webhook", "removed", []byte(`{"name":"my-hook","url":"https://hooks.example.com/1"}`), nil)
	if !strings.Contains(s, "my-hook") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "my-hook" {
		t.Errorf("target=%q", target)
	}
}

func TestWebhookChanged(t *testing.T) {
	s, _ := Render("webhook", "changed",
		[]byte(`{"name":"h","url":"https://old.example.com","events":["deploy"]}`),
		[]byte(`{"name":"h","url":"https://new.example.com","events":["deploy"]}`))
	if !strings.Contains(s, "url") || !strings.Contains(s, "new.example.com") {
		t.Errorf("expected url diff, got %q", s)
	}
}

func TestWebhookChangedFallback(t *testing.T) {
	s, _ := Render("webhook", "changed",
		[]byte(`{"name":"h","url":"https://x.com","events":["deploy"]}`),
		[]byte(`{"name":"h","url":"https://x.com","events":["deploy"]}`))
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated, got %q", s)
	}
}

// --- registry ---

func TestRegistryAdded(t *testing.T) {
	s, target := Render("registry", "added", nil, []byte(`{"name":"ghcr","url":"ghcr.io"}`))
	if !strings.Contains(s, "ghcr") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "ghcr" {
		t.Errorf("target=%q", target)
	}
}

func TestRegistryRemoved(t *testing.T) {
	s, target := Render("registry", "removed", []byte(`{"name":"ghcr","url":"ghcr.io"}`), nil)
	if !strings.Contains(s, "ghcr") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "ghcr" {
		t.Errorf("target=%q", target)
	}
}

func TestRegistryChanged(t *testing.T) {
	s, _ := Render("registry", "changed",
		[]byte(`{"name":"r","url":"old.registry.io"}`),
		[]byte(`{"name":"r","url":"new.registry.io"}`))
	if !strings.Contains(s, "url") || !strings.Contains(s, "new.registry.io") {
		t.Errorf("expected url diff, got %q", s)
	}
}

func TestRegistryChangedFallback(t *testing.T) {
	s, _ := Render("registry", "changed",
		[]byte(`{"name":"r","url":"x.io"}`),
		[]byte(`{"name":"r","url":"x.io"}`))
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated, got %q", s)
	}
}

// --- access ---

func TestAccessAdded(t *testing.T) {
	s, target := Render("access", "added", nil, []byte(`{"username":"alice","role":"viewer"}`))
	if !strings.Contains(s, "alice") || !strings.Contains(s, "granted") {
		t.Errorf("unexpected summary: %q", s)
	}
	if !strings.Contains(s, "viewer") {
		t.Errorf("expected role in summary: %q", s)
	}
	if target != "alice" {
		t.Errorf("target=%q", target)
	}
}

func TestAccessRemoved(t *testing.T) {
	s, target := Render("access", "removed", []byte(`{"username":"bob","role":"admin"}`), nil)
	if !strings.Contains(s, "bob") || !strings.Contains(s, "revoked") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "bob" {
		t.Errorf("target=%q", target)
	}
}

func TestAccessChanged(t *testing.T) {
	s, target := Render("access", "changed",
		[]byte(`{"username":"carol","role":"viewer"}`),
		[]byte(`{"username":"carol","role":"admin"}`))
	if !strings.Contains(s, "carol") || !strings.Contains(s, "viewer") || !strings.Contains(s, "admin") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "carol" {
		t.Errorf("target=%q", target)
	}
}

// --- deploy ---

func TestDeploySucceeded(t *testing.T) {
	s, target := Render("deploy", "deploy_succeeded", nil, []byte(`{"version":7}`))
	if !strings.Contains(s, "succeeded") || !strings.Contains(s, "7") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

func TestDeployFailed(t *testing.T) {
	s, _ := Render("deploy", "deploy_failed", nil, []byte(`{"version":42,"error":"image pull denied"}`))
	if !strings.Contains(s, "failed") || !strings.Contains(s, "image pull denied") {
		t.Errorf("unexpected: %q", s)
	}
}

func TestDeployRollback(t *testing.T) {
	s, target := Render("deploy", "rollback", nil, []byte(`{"version":5}`))
	if !strings.Contains(s, "Rolled back") || !strings.Contains(s, "5") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

// --- lifecycle ---

func TestLifecycleCreated(t *testing.T) {
	s, target := Render("lifecycle", "created", nil, []byte(`{"name":"myapp"}`))
	if !strings.Contains(s, "myapp") || !strings.Contains(s, "created") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}

func TestLifecycleRenamed(t *testing.T) {
	s, target := Render("lifecycle", "renamed",
		[]byte(`{"name":"old-app"}`),
		[]byte(`{"name":"new-app"}`))
	if !strings.Contains(s, "old-app") || !strings.Contains(s, "new-app") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "new-app" {
		t.Errorf("target=%q", target)
	}
}

func TestLifecycleRemoved(t *testing.T) {
	s, target := Render("lifecycle", "removed", []byte(`{"name":"gone-app"}`), nil)
	if !strings.Contains(s, "gone-app") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "gone-app" {
		t.Errorf("target=%q", target)
	}
}

func TestLifecycleStopped(t *testing.T) {
	s, target := Render("lifecycle", "stopped", nil, []byte(`{"name":"myapp"}`))
	if !strings.Contains(s, "myapp") || !strings.Contains(s, "stopped") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}

func TestLifecycleStarted(t *testing.T) {
	s, target := Render("lifecycle", "started", nil, []byte(`{"name":"myapp"}`))
	if !strings.Contains(s, "myapp") || !strings.Contains(s, "started") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}

func TestLifecycleScaled(t *testing.T) {
	s, target := Render("lifecycle", "scaled",
		[]byte(`{"name":"myapp","replicas":1}`),
		[]byte(`{"name":"myapp","replicas":3}`))
	if !strings.Contains(s, "myapp") || !strings.Contains(s, "scaled") {
		t.Errorf("unexpected summary: %q", s)
	}
	if !strings.Contains(s, "1") || !strings.Contains(s, "3") {
		t.Errorf("expected replica counts in summary: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}

// --- auth ---

func TestAuthLoginSucceeded(t *testing.T) {
	s, target := Render("auth", "login_succeeded", nil, nil)
	if s != "Login succeeded" {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

func TestAuthLoginFailed(t *testing.T) {
	s, target := Render("auth", "login_failed", nil, nil)
	if s != "Login failed" {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

func TestAuthPasswordChanged(t *testing.T) {
	s, target := Render("auth", "password_changed", nil, nil)
	if s != "Password changed" {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

// --- system ---

func TestSystemUserAdded(t *testing.T) {
	s, target := Render("system", "user_added", nil, []byte(`{"username":"dave","role":"admin"}`))
	if !strings.Contains(s, "dave") || !strings.Contains(s, "added") {
		t.Errorf("unexpected summary: %q", s)
	}
	if !strings.Contains(s, "admin") {
		t.Errorf("expected role in summary: %q", s)
	}
	if target != "dave" {
		t.Errorf("target=%q", target)
	}
}

func TestSystemUserChanged(t *testing.T) {
	s, target := Render("system", "user_changed",
		[]byte(`{"username":"dave","role":"viewer"}`),
		[]byte(`{"username":"dave","role":"admin"}`))
	if !strings.Contains(s, "dave") || !strings.Contains(s, "viewer") || !strings.Contains(s, "admin") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "dave" {
		t.Errorf("target=%q", target)
	}
}

func TestSystemUserRemoved(t *testing.T) {
	s, target := Render("system", "user_removed", []byte(`{"username":"eve"}`), nil)
	if !strings.Contains(s, "eve") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "eve" {
		t.Errorf("target=%q", target)
	}
}

func TestSystemApikeyAdded(t *testing.T) {
	s, target := Render("system", "apikey_added", nil, []byte(`{"name":"ci-key","username":"frank"}`))
	if !strings.Contains(s, "ci-key") || !strings.Contains(s, "frank") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "frank" {
		t.Errorf("target=%q", target)
	}
}

func TestSystemApikeyRemoved(t *testing.T) {
	s, _ := Render("system", "apikey_removed", []byte(`{"name":"ci-key","username":"frank"}`), nil)
	if !strings.Contains(s, "ci-key") || !strings.Contains(s, "removed") {
		t.Errorf("unexpected summary: %q", s)
	}
}

func TestSystemPublicHostChanged(t *testing.T) {
	s, target := Render("system", "public_host_changed", nil, []byte(`{"host":"deploy.example.com"}`))
	if !strings.Contains(s, "deploy.example.com") {
		t.Errorf("unexpected summary: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

// --- env ---

func TestEnvChangedAddedRemoved(t *testing.T) {
	before := []byte(`{"keys":["A","B"]}`)
	after := []byte(`{"keys":["A","C"]}`)
	s, _ := Render("env", "changed", before, after)
	if !strings.Contains(s, "added C") {
		t.Errorf("expected added C: %q", s)
	}
	if !strings.Contains(s, "removed B") {
		t.Errorf("expected removed B: %q", s)
	}
	// Values must never appear in summary.
	if strings.Contains(s, "secret") {
		t.Errorf("summary leaked value: %q", s)
	}
}

func TestEnvChangedNoOp(t *testing.T) {
	before := []byte(`{"keys":["A"]}`)
	after := []byte(`{"keys":["A"]}`)
	s, _ := Render("env", "changed", before, after)
	if !strings.Contains(s, "updated") {
		t.Errorf("expected fallback updated: %q", s)
	}
}

// --- access iplist_changed ---

func TestAccessIPListChanged(t *testing.T) {
	before := []byte(`{"allow":["1.1.1.1","2.2.2.2"]}`)
	after := []byte(`{"allow":["1.1.1.1","3.3.3.3","4.4.4.4"]}`)
	s, _ := Render("access", "iplist_changed", before, after)
	if !strings.Contains(s, "2 entries added") || !strings.Contains(s, "1 removed") {
		t.Errorf("unexpected summary: %q", s)
	}
}

// --- endpoint cert ---

func TestEndpointCertUploaded(t *testing.T) {
	s, target := Render("endpoint", "cert_uploaded", nil, []byte(`{"domain":"foo.example.com"}`))
	if !strings.Contains(s, "uploaded") || !strings.Contains(s, "foo.example.com") {
		t.Errorf("unexpected: %q", s)
	}
	if target != "foo.example.com" {
		t.Errorf("target=%q", target)
	}
}

func TestEndpointCertRemoved(t *testing.T) {
	s, target := Render("endpoint", "cert_removed", []byte(`{"domain":"foo.example.com"}`), nil)
	if !strings.Contains(s, "removed") || !strings.Contains(s, "foo.example.com") {
		t.Errorf("unexpected: %q", s)
	}
	if target != "foo.example.com" {
		t.Errorf("target=%q", target)
	}
}

// --- lifecycle image_pulled ---

func TestLifecycleImagePulled(t *testing.T) {
	s, target := Render("lifecycle", "image_pulled", nil, []byte(`{"name":"myapp"}`))
	if !strings.Contains(s, "Images pulled") || !strings.Contains(s, "myapp") {
		t.Errorf("unexpected: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}

// --- compose version_removed ---

func TestComposeVersionRemoved(t *testing.T) {
	s, _ := Render("compose", "version_removed", []byte(`{"version":7}`), nil)
	if !strings.Contains(s, "version 7 removed") {
		t.Errorf("unexpected: %q", s)
	}
}

// --- deploy cancelled ---

func TestDeployCancelled(t *testing.T) {
	s, target := Render("deploy", "cancelled", nil, []byte(`{"name":"myapp"}`))
	if !strings.Contains(s, "cancelled") || !strings.Contains(s, "myapp") {
		t.Errorf("unexpected: %q", s)
	}
	if target != "myapp" {
		t.Errorf("target=%q", target)
	}
}
