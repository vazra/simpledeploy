package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRecordAndGetAudit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e := AuditEntry{
		ActorSource:  "api",
		Category:     "compose",
		Action:       "edit",
		Summary:      "edited compose file",
		SyncEligible: true,
	}
	id, err := s.RecordAudit(ctx, e)
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := s.GetActivity(ctx, id)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if got.Summary != "edited compose file" {
		t.Errorf("summary = %q, want %q", got.Summary, "edited compose file")
	}
	if got.SyncStatus == nil || *got.SyncStatus != "pending" {
		t.Errorf("sync_status = %v, want pending", got.SyncStatus)
	}
}

func TestListActivityFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, cat := range []string{"compose", "deploy", "compose"} {
		_, err := s.RecordAudit(ctx, AuditEntry{
			ActorSource: "api",
			Category:    cat,
			Action:      "test",
			Summary:     cat + " action",
		})
		if err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
	}

	entries, _, err := s.ListActivity(ctx, ActivityFilter{
		Categories: []string{"compose"},
		Limit:      50,
	})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Category != "compose" {
			t.Errorf("unexpected category %q", e.Category)
		}
	}
}

func TestListActivityPagination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		_, err := s.RecordAudit(ctx, AuditEntry{
			ActorSource: "api",
			Category:    "deploy",
			Action:      "start",
			Summary:     fmt.Sprintf("entry %d", i),
		})
		if err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
	}

	page1, nextBefore, err := s.ListActivity(ctx, ActivityFilter{Limit: 25})
	if err != nil {
		t.Fatalf("ListActivity page1: %v", err)
	}
	if len(page1) != 25 {
		t.Errorf("page1 len = %d, want 25", len(page1))
	}
	if nextBefore == 0 {
		t.Error("nextBefore should be > 0")
	}

	page2, _, err := s.ListActivity(ctx, ActivityFilter{Before: nextBefore, Limit: 25})
	if err != nil {
		t.Fatalf("ListActivity page2: %v", err)
	}
	if len(page2) != 25 {
		t.Errorf("page2 len = %d, want 25", len(page2))
	}

	// verify no overlap
	ids1 := make(map[int64]bool)
	for _, e := range page1 {
		ids1[e.ID] = true
	}
	for _, e := range page2 {
		if ids1[e.ID] {
			t.Errorf("id %d appears in both pages", e.ID)
		}
	}
}

func TestListActivityAllowedApps(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert app rows so FK constraints are satisfied.
	_, err := s.db.Exec(`INSERT INTO apps (id, name, slug, compose_path, status) VALUES (1, 'app1', 'app1', '/apps/app1/docker-compose.yml', 'running')`)
	if err != nil {
		t.Fatalf("insert app1: %v", err)
	}
	_, err = s.db.Exec(`INSERT INTO apps (id, name, slug, compose_path, status) VALUES (2, 'app2', 'app2', '/apps/app2/docker-compose.yml', 'running')`)
	if err != nil {
		t.Fatalf("insert app2: %v", err)
	}

	appID1 := int64(1)
	appID2 := int64(2)

	entries := []AuditEntry{
		{AppID: &appID1, AppSlug: "app1", ActorSource: "api", Category: "deploy", Action: "start", Summary: "app1 event"},
		{AppID: &appID2, AppSlug: "app2", ActorSource: "api", Category: "deploy", Action: "start", Summary: "app2 event"},
		{ActorSource: "system", Category: "system", Action: "audit", Summary: "system event"},
	}
	for _, e := range entries {
		if _, err := s.RecordAudit(ctx, e); err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
	}

	// AllowedAppIDs=[1] -> app1 rows + system (app_id IS NULL)
	got, _, err := s.ListActivity(ctx, ActivityFilter{
		AllowedAppIDs: []int64{1},
		Limit:         50,
	})
	if err != nil {
		t.Fatalf("ListActivity allowed=[1]: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("allowed=[1]: got %d, want 2", len(got))
	}
	for _, e := range got {
		if e.AppID != nil && *e.AppID != 1 {
			t.Errorf("unexpected app_id %d in result", *e.AppID)
		}
	}

	// AllowedAppIDs=[] -> only app_id IS NULL
	got2, _, err := s.ListActivity(ctx, ActivityFilter{
		AllowedAppIDs: []int64{},
		Limit:         50,
	})
	if err != nil {
		t.Fatalf("ListActivity allowed=[]: %v", err)
	}
	if len(got2) != 1 {
		t.Errorf("allowed=[]: got %d, want 1", len(got2))
	}
	if got2[0].AppID != nil {
		t.Errorf("expected app_id IS NULL, got %d", *got2[0].AppID)
	}
}

func TestPruneAudit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.RecordAudit(ctx, AuditEntry{
		ActorSource: "api",
		Category:    "deploy",
		Action:      "start",
		Summary:     "old event",
	})
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	n, err := s.PruneAudit(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("PruneAudit: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted %d rows, want 1", n)
	}

	entries, _, err := s.ListActivity(ctx, ActivityFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after prune, got %d", len(entries))
	}
}

func TestSyncTransitions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert pending entry.
	id, err := s.RecordAudit(ctx, AuditEntry{
		ActorSource:  "api",
		Category:     "compose",
		Action:       "edit",
		Summary:      "sync test",
		SyncEligible: true,
	})
	if err != nil {
		t.Fatalf("RecordAudit: %v", err)
	}

	ids, err := s.PendingSyncAuditIDs(ctx)
	if err != nil {
		t.Fatalf("PendingSyncAuditIDs: %v", err)
	}
	if len(ids) != 1 || ids[0] != id {
		t.Errorf("PendingSyncAuditIDs = %v, want [%d]", ids, id)
	}

	const sha = "abc123def456"
	if err := s.MarkSyncSynced(ctx, []int64{id}, sha); err != nil {
		t.Fatalf("MarkSyncSynced: %v", err)
	}

	got, err := s.GetActivity(ctx, id)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if got.SyncStatus == nil || *got.SyncStatus != "synced" {
		t.Errorf("sync_status = %v, want synced", got.SyncStatus)
	}
	if got.SyncCommitSHA != sha {
		t.Errorf("commit SHA = %q, want %q", got.SyncCommitSHA, sha)
	}

	// Another entry -> MarkSyncFailed.
	id2, err := s.RecordAudit(ctx, AuditEntry{
		ActorSource:  "api",
		Category:     "compose",
		Action:       "edit",
		Summary:      "sync fail test",
		SyncEligible: true,
	})
	if err != nil {
		t.Fatalf("RecordAudit2: %v", err)
	}

	const errMsg = "push failed: conflict"
	if err := s.MarkSyncFailed(ctx, []int64{id2}, errMsg); err != nil {
		t.Fatalf("MarkSyncFailed: %v", err)
	}

	got2, err := s.GetActivity(ctx, id2)
	if err != nil {
		t.Fatalf("GetActivity2: %v", err)
	}
	if got2.SyncStatus == nil || *got2.SyncStatus != "failed" {
		t.Errorf("sync_status = %v, want failed", got2.SyncStatus)
	}
	if got2.SyncError != errMsg {
		t.Errorf("sync_error = %q, want %q", got2.SyncError, errMsg)
	}
}

func TestPurgeAudit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, err := s.RecordAudit(ctx, AuditEntry{
			ActorSource: "api",
			Category:    "deploy",
			Action:      "start",
			Summary:     fmt.Sprintf("entry %d", i),
		})
		if err != nil {
			t.Fatalf("RecordAudit: %v", err)
		}
	}

	if err := s.PurgeAudit(ctx); err != nil {
		t.Fatalf("PurgeAudit: %v", err)
	}

	entries, _, err := s.ListActivity(ctx, ActivityFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after purge, got %d", len(entries))
	}
}

func TestRetentionConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	days, err := s.GetAuditRetentionDays(ctx)
	if err != nil {
		t.Fatalf("GetAuditRetentionDays default: %v", err)
	}
	if days != 365 {
		t.Errorf("default retention = %d, want 365", days)
	}

	if err := s.SetAuditRetentionDays(ctx, 90); err != nil {
		t.Fatalf("SetAuditRetentionDays: %v", err)
	}

	days, err = s.GetAuditRetentionDays(ctx)
	if err != nil {
		t.Fatalf("GetAuditRetentionDays after set: %v", err)
	}
	if days != 90 {
		t.Errorf("retention after set = %d, want 90", days)
	}
}
