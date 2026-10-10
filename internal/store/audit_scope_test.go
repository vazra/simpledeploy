package store

import (
	"context"
	"sort"
	"testing"
)

func TestListActivityCurrentAppID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rec := func(e AuditEntry) int64 {
		t.Helper()
		e.ActorSource = "ui"
		if e.Summary == "" {
			e.Summary = e.Category + "/" + e.Action
		}
		id, err := s.RecordAudit(ctx, e)
		if err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
		return id
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	ids := func(f ActivityFilter) []int64 {
		t.Helper()
		f.Limit = 200
		got, _, err := s.ListActivity(ctx, f)
		if err != nil {
			t.Fatalf("ListActivity: %v", err)
		}
		out := make([]int64, 0, len(got))
		for _, e := range got {
			out = append(out, e.ID)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}

	// Old incarnation of "reused": created, deployed, purged two hours ago.
	oldCreated := rec(AuditEntry{AppSlug: "reused", Category: "lifecycle", Action: "created"})
	exec(`INSERT INTO apps (id, name, slug, compose_path, status, created_at) VALUES (1, 'reused', 'reused', '/a/reused/docker-compose.yml', 'running', datetime('now', '-3 hours'))`)
	oldApp := int64(1)
	oldDeploy := rec(AuditEntry{AppID: &oldApp, AppSlug: "reused", Category: "deploy", Action: "deploy_succeeded"})
	oldEnv := rec(AuditEntry{AppSlug: "reused", Category: "env", Action: "changed"})
	if err := s.PurgeApp("reused"); err != nil {
		t.Fatal(err)
	}
	oldPurged := rec(AuditEntry{AppSlug: "reused", Category: "lifecycle", Action: "purged"})
	exec(`UPDATE audit_log SET created_at = datetime('now', '-2 hours') WHERE id IN (?, ?, ?, ?)`, oldCreated, oldDeploy, oldEnv, oldPurged)

	// New incarnation: lifecycle/created and the first compose change are
	// recorded before the reconciler inserts the apps row (after a slow
	// image pull), so they predate apps.created_at.
	newCreated := rec(AuditEntry{AppSlug: "reused", Category: "lifecycle", Action: "created"})
	newCompose := rec(AuditEntry{AppSlug: "reused", Category: "compose", Action: "changed"})
	exec(`UPDATE audit_log SET created_at = datetime('now', '-5 minutes') WHERE id IN (?, ?)`, newCreated, newCompose)
	exec(`INSERT INTO apps (id, name, slug, compose_path, status) VALUES (2, 'reused', 'reused', '/a/reused/docker-compose.yml', 'running')`)
	newApp := int64(2)
	newDeploy := rec(AuditEntry{AppID: &newApp, AppSlug: "reused", Category: "deploy", Action: "deploy_succeeded"})
	newSlugOnly := rec(AuditEntry{AppSlug: "reused", Category: "env", Action: "changed"})
	// Unrelated app row must never show up.
	rec(AuditEntry{AppSlug: "other", Category: "env", Action: "changed"})

	got := ids(ActivityFilter{AppSlug: "reused", CurrentAppID: &newApp})
	want := []int64{newCreated, newCompose, newDeploy, newSlugOnly}
	if !equalAuditIDs(got, want) {
		t.Fatalf("scoped ids = %v, want %v (old: %v)", got, want, []int64{oldCreated, oldDeploy, oldEnv, oldPurged})
	}

	// Unscoped (super_admin) keeps the full slug history.
	if all := ids(ActivityFilter{AppSlug: "reused"}); len(all) != 8 {
		t.Fatalf("unscoped got %d rows, want 8", len(all))
	}
}

func TestListActivityCurrentAppIDWithoutCreatedEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rec := func(e AuditEntry) int64 {
		t.Helper()
		e.ActorSource = "system"
		e.Summary = "x"
		id, err := s.RecordAudit(ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// Previous incarnation was created via the API and purged.
	oldCreated := rec(AuditEntry{AppSlug: "adopted", Category: "lifecycle", Action: "created"})
	oldPurged := rec(AuditEntry{AppSlug: "adopted", Category: "lifecycle", Action: "purged"})
	if _, err := s.db.Exec(`UPDATE audit_log SET created_at = datetime('now', '-2 hours') WHERE id IN (?, ?)`, oldCreated, oldPurged); err != nil {
		t.Fatal(err)
	}
	// The current app was adopted from disk (no lifecycle/created row): the
	// old created row must not act as its boundary.
	if _, err := s.db.Exec(`INSERT INTO apps (id, name, slug, compose_path, status, created_at) VALUES (5, 'adopted', 'adopted', '/a/adopted/docker-compose.yml', 'running', datetime('now', '-1 hour'))`); err != nil {
		t.Fatal(err)
	}
	appID := int64(5)
	after := rec(AuditEntry{AppSlug: "adopted", Category: "env", Action: "changed"})
	own := rec(AuditEntry{AppID: &appID, AppSlug: "adopted", Category: "deploy", Action: "deploy_succeeded"})

	got, _, err := s.ListActivity(ctx, ActivityFilter{AppSlug: "adopted", CurrentAppID: &appID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var gotIDs []int64
	for _, e := range got {
		gotIDs = append(gotIDs, e.ID)
	}
	sort.Slice(gotIDs, func(i, j int) bool { return gotIDs[i] < gotIDs[j] })
	if !equalAuditIDs(gotIDs, []int64{after, own}) {
		t.Fatalf("ids = %v, want %v", gotIDs, []int64{after, own})
	}
}

func equalAuditIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
