package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/store"
)

func TestDeployerAuditAdapterVersion(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a := &DeployerAuditAdapter{Rec: audit.NewRecorder(st)}
	ctx := context.Background()

	// Deployer.Deploy leaves Version unset: it runs before the reconciler
	// stores the compose version.
	a.RecordDeploy(ctx, deployer.DeployAuditEvent{AppSlug: "web", Action: "deploy_succeeded"})
	a.RecordDeploy(ctx, deployer.DeployAuditEvent{AppSlug: "web", Action: "rollback", Version: 3})

	entries, _, err := st.ListActivity(ctx, store.ActivityFilter{AppSlug: "web", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]store.AuditEntry{}
	for _, e := range entries {
		got[e.Action] = e
	}
	if e := got["deploy_succeeded"]; e.Summary != "Deploy succeeded" {
		t.Errorf("deploy_succeeded summary = %q, want %q", e.Summary, "Deploy succeeded")
	}
	if e := got["rollback"]; e.Summary != "Rolled back to version 3" {
		t.Errorf("rollback summary = %q", e.Summary)
	}
}
