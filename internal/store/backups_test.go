package store

import (
	"testing"
)

func makeTestApp(t *testing.T, s *Store) *App {
	t.Helper()
	app := &App{
		Name:        "test-app",
		Slug:        "test-app",
		ComposePath: "/tmp/test/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}
	return app
}

func makeTestBackupConfig(t *testing.T, s *Store, appID int64) *BackupConfig {
	t.Helper()
	cfg := &BackupConfig{
		AppID:            appID,
		Strategy:         "postgres",
		Target:           "s3",
		ScheduleCron:     "0 2 * * *",
		TargetConfigJSON: `{"bucket":"mybucket"}`,
		RetentionMode:    "count",
		RetentionCount:   7,
	}
	if err := s.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("CreateBackupConfig: %v", err)
	}
	return cfg
}

func TestBackupConfigCRUD(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)

	days := 30
	cfg := &BackupConfig{
		AppID:            app.ID,
		Strategy:         "volume",
		Target:           "local",
		ScheduleCron:     "0 3 * * *",
		TargetConfigJSON: `{"path":"/backups"}`,
		RetentionMode:    "time",
		RetentionCount:   0,
		RetentionDays:    &days,
		VerifyUpload:     true,
		PreHooks:         "echo pre",
		PostHooks:        "echo post",
		Paths:            "/data,/config",
	}
	if err := s.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("CreateBackupConfig: %v", err)
	}
	if cfg.ID == 0 {
		t.Fatal("expected ID to be set after create")
	}
	if cfg.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set after create")
	}
	if cfg.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set after create")
	}

	// list all
	all, err := s.ListBackupConfigs(nil)
	if err != nil {
		t.Fatalf("ListBackupConfigs(nil): %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("len(all) = %d, want 1", len(all))
	}

	// list by app
	byApp, err := s.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("ListBackupConfigs(appID): %v", err)
	}
	if len(byApp) != 1 {
		t.Fatalf("len(byApp) = %d, want 1", len(byApp))
	}
	if byApp[0].Strategy != "volume" {
		t.Errorf("Strategy = %q, want volume", byApp[0].Strategy)
	}

	// get and verify all new fields
	got, err := s.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatalf("GetBackupConfig: %v", err)
	}
	if got.Target != "local" {
		t.Errorf("Target = %q, want local", got.Target)
	}
	if got.RetentionMode != "time" {
		t.Errorf("RetentionMode = %q, want time", got.RetentionMode)
	}
	if got.RetentionCount != 0 {
		t.Errorf("RetentionCount = %d, want 0", got.RetentionCount)
	}
	if got.RetentionDays == nil || *got.RetentionDays != 30 {
		t.Errorf("RetentionDays = %v, want 30", got.RetentionDays)
	}
	if !got.VerifyUpload {
		t.Error("VerifyUpload = false, want true")
	}
	if got.PreHooks != "echo pre" {
		t.Errorf("PreHooks = %q, want 'echo pre'", got.PreHooks)
	}
	if got.PostHooks != "echo post" {
		t.Errorf("PostHooks = %q, want 'echo post'", got.PostHooks)
	}
	if got.Paths != "/data,/config" {
		t.Errorf("Paths = %q, want '/data,/config'", got.Paths)
	}

	// delete
	if err := s.DeleteBackupConfig(cfg.ID); err != nil {
		t.Fatalf("DeleteBackupConfig: %v", err)
	}
	all, err = s.ListBackupConfigs(nil)
	if err != nil {
		t.Fatalf("ListBackupConfigs after delete: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("len(all) = %d, want 0 after delete", len(all))
	}
	if _, err := s.GetBackupConfig(cfg.ID); err == nil {
		t.Fatal("expected error getting deleted config, got nil")
	}
}

func TestUpdateBackupConfig(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	// switch from count to time retention
	days := 14
	cfg.RetentionMode = "time"
	cfg.RetentionCount = 0
	cfg.RetentionDays = &days
	cfg.VerifyUpload = true
	cfg.PreHooks = "pg_dump check"

	if err := s.UpdateBackupConfig(cfg); err != nil {
		t.Fatalf("UpdateBackupConfig: %v", err)
	}

	got, err := s.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatalf("GetBackupConfig: %v", err)
	}
	if got.RetentionMode != "time" {
		t.Errorf("RetentionMode = %q, want time", got.RetentionMode)
	}
	if got.RetentionDays == nil || *got.RetentionDays != 14 {
		t.Errorf("RetentionDays = %v, want 14", got.RetentionDays)
	}
	if !got.VerifyUpload {
		t.Error("VerifyUpload = false, want true")
	}
	if got.PreHooks != "pg_dump check" {
		t.Errorf("PreHooks = %q, want 'pg_dump check'", got.PreHooks)
	}
	// app_id and created_at should be unchanged
	if got.AppID != app.ID {
		t.Errorf("AppID changed unexpectedly")
	}
}

func TestBackupRunLifecycle(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	run, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("expected ID to be set")
	}
	if run.Status != "running" {
		t.Errorf("Status = %q, want running", run.Status)
	}
	if run.SizeBytes != nil {
		t.Error("SizeBytes should be nil while running")
	}
	if run.FinishedAt != nil {
		t.Error("FinishedAt should be nil while running")
	}
	if run.StartedAt.IsZero() {
		t.Fatal("expected StartedAt to be set")
	}

	var size int64 = 1024 * 1024
	if err := s.UpdateBackupRunSuccess(run.ID, size, "/backups/dump.sql.gz", "sha256:abc123"); err != nil {
		t.Fatalf("UpdateBackupRunSuccess: %v", err)
	}

	got, err := s.GetBackupRun(run.ID)
	if err != nil {
		t.Fatalf("GetBackupRun: %v", err)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.SizeBytes == nil {
		t.Fatal("expected SizeBytes to be set after success")
	}
	if *got.SizeBytes != size {
		t.Errorf("SizeBytes = %d, want %d", *got.SizeBytes, size)
	}
	if got.FilePath != "/backups/dump.sql.gz" {
		t.Errorf("FilePath = %q, want /backups/dump.sql.gz", got.FilePath)
	}
	if got.Checksum != "sha256:abc123" {
		t.Errorf("Checksum = %q, want sha256:abc123", got.Checksum)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set after success")
	}
}

func TestBackupRunFailed(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	run, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}

	if err := s.UpdateBackupRunFailed(run.ID, "connection refused"); err != nil {
		t.Fatalf("UpdateBackupRunFailed: %v", err)
	}

	got, err := s.GetBackupRun(run.ID)
	if err != nil {
		t.Fatalf("GetBackupRun: %v", err)
	}
	if got.Status != "failed" {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.ErrorMsg != "connection refused" {
		t.Errorf("ErrorMsg = %q, want connection refused", got.ErrorMsg)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be set after failure")
	}
	if got.SizeBytes != nil {
		t.Error("SizeBytes should be nil after failure")
	}
}

func TestListOldBackupRuns(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	// create 5 successful runs
	for i := 0; i < 5; i++ {
		run, err := s.CreateBackupRun(cfg.ID)
		if err != nil {
			t.Fatalf("CreateBackupRun %d: %v", i, err)
		}
		if err := s.UpdateBackupRunSuccess(run.ID, int64(i*100), "/backups/dump.sql.gz", "sha256:test"); err != nil {
			t.Fatalf("UpdateBackupRunSuccess %d: %v", i, err)
		}
	}

	old, err := s.ListOldBackupRuns(cfg.ID, 3)
	if err != nil {
		t.Fatalf("ListOldBackupRuns: %v", err)
	}
	if len(old) != 2 {
		t.Fatalf("len(old) = %d, want 2", len(old))
	}
	for _, r := range old {
		if r.Status != "success" {
			t.Errorf("Status = %q, want success", r.Status)
		}
	}
}

func TestListOldBackupRunsByTime(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	// create a run and mark as success
	run, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}
	if err := s.UpdateBackupRunSuccess(run.ID, 500, "/backups/old.sql.gz", "sha256:old"); err != nil {
		t.Fatalf("UpdateBackupRunSuccess: %v", err)
	}

	// manually backdate the run's started_at to 10 days ago
	_, err = s.db.Exec(`UPDATE backup_runs SET started_at = datetime('now', '-10 days') WHERE id = ?`, run.ID)
	if err != nil {
		t.Fatalf("backdate run: %v", err)
	}

	// create a recent run
	run2, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}
	if err := s.UpdateBackupRunSuccess(run2.ID, 600, "/backups/new.sql.gz", "sha256:new"); err != nil {
		t.Fatalf("UpdateBackupRunSuccess: %v", err)
	}

	// 7-day cutoff should return the old run only
	old, err := s.ListOldBackupRunsByTime(cfg.ID, 7)
	if err != nil {
		t.Fatalf("ListOldBackupRunsByTime: %v", err)
	}
	if len(old) != 1 {
		t.Fatalf("len(old) = %d, want 1", len(old))
	}
	if old[0].ID != run.ID {
		t.Errorf("got run ID %d, want %d", old[0].ID, run.ID)
	}

	// 30-day cutoff should return nothing (both runs within 30 days)
	old, err = s.ListOldBackupRunsByTime(cfg.ID, 30)
	if err != nil {
		t.Fatalf("ListOldBackupRunsByTime(30): %v", err)
	}
	if len(old) != 0 {
		t.Errorf("len(old) = %d, want 0 for 30-day cutoff", len(old))
	}
}

func TestGetBackupSummary(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	run, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}
	var size int64 = 2048
	if err := s.UpdateBackupRunSuccess(run.ID, size, "/backups/dump.sql.gz", "sha256:sum"); err != nil {
		t.Fatalf("UpdateBackupRunSuccess: %v", err)
	}

	summaries, err := s.GetBackupSummary()
	if err != nil {
		t.Fatalf("GetBackupSummary: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	got := summaries[0]
	if got.AppSlug != app.Slug {
		t.Errorf("AppSlug = %q, want %q", got.AppSlug, app.Slug)
	}
	if got.ConfigCount != 1 {
		t.Errorf("ConfigCount = %d, want 1", got.ConfigCount)
	}
	if got.TotalSizeBytes != size {
		t.Errorf("TotalSizeBytes = %d, want %d", got.TotalSizeBytes, size)
	}
	if got.LastRunStatus != "success" {
		t.Errorf("LastRunStatus = %q, want success", got.LastRunStatus)
	}
	if got.RecentSuccessCount != 1 {
		t.Errorf("RecentSuccessCount = %d, want 1", got.RecentSuccessCount)
	}
}

func TestListRecentBackupRuns(t *testing.T) {
	s := newTestStore(t)
	app := makeTestApp(t, s)
	cfg := makeTestBackupConfig(t, s, app.ID)

	run, err := s.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatalf("CreateBackupRun: %v", err)
	}
	if err := s.UpdateBackupRunSuccess(run.ID, 512, "/backups/dump.sql.gz", "sha256:recent"); err != nil {
		t.Fatalf("UpdateBackupRunSuccess: %v", err)
	}

	runs, err := s.ListRecentBackupRuns(10)
	if err != nil {
		t.Fatalf("ListRecentBackupRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	got := runs[0]
	if got.AppName != app.Name {
		t.Errorf("AppName = %q, want %q", got.AppName, app.Name)
	}
	if got.AppSlug != app.Slug {
		t.Errorf("AppSlug = %q, want %q", got.AppSlug, app.Slug)
	}
	if got.Strategy != cfg.Strategy {
		t.Errorf("Strategy = %q, want %q", got.Strategy, cfg.Strategy)
	}
	if got.Status != "success" {
		t.Errorf("Status = %q, want success", got.Status)
	}
	if got.Checksum != "sha256:recent" {
		t.Errorf("Checksum = %q, want sha256:recent", got.Checksum)
	}
}
