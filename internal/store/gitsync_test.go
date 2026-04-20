package store

import (
	"path/filepath"
	"testing"
)

func TestInsertConflictAlert(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	err = s.InsertConflictAlert("app1/simpledeploy.yml", "abc123", "server-wins")
	if err != nil {
		t.Fatalf("InsertConflictAlert: %v", err)
	}

	hist, err := s.ListAlertHistory(nil, 10)
	if err != nil {
		t.Fatalf("ListAlertHistory: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("got %d rows, want 1", len(hist))
	}
	h := hist[0]
	if h.RuleID != nil {
		t.Errorf("RuleID = %v, want nil", *h.RuleID)
	}
	if h.Metric != GitSyncConflictMetric {
		t.Errorf("Metric = %q, want %q", h.Metric, GitSyncConflictMetric)
	}
	if h.AppSlug != "app1/simpledeploy.yml" {
		t.Errorf("AppSlug = %q, want %q", h.AppSlug, "app1/simpledeploy.yml")
	}
	if h.Operator != "server-wins" {
		t.Errorf("Operator = %q, want %q", h.Operator, "server-wins")
	}
}
