package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// --- mock store ---

type mockStore struct {
	configs     map[int64]*store.BackupConfig
	runs        map[int64]*store.BackupRun
	nextRunID   int64
	successCall *successArgs
	failedCall  *failedArgs
	oldRuns     []store.BackupRun
	oldRunsTime []store.BackupRun
	deletedRuns []int64
}

type successArgs struct {
	id        int64
	sizeBytes int64
	filePath  string
	checksum  string
}

type failedArgs struct {
	id     int64
	errMsg string
}

func newMockStore() *mockStore {
	return &mockStore{
		configs:   make(map[int64]*store.BackupConfig),
		runs:      make(map[int64]*store.BackupRun),
		nextRunID: 1,
	}
}

func (m *mockStore) ListBackupConfigs(appID *int64) ([]store.BackupConfig, error) {
	var out []store.BackupConfig
	for _, c := range m.configs {
		out = append(out, *c)
	}
	return out, nil
}

func (m *mockStore) GetBackupConfig(id int64) (*store.BackupConfig, error) {
	c, ok := m.configs[id]
	if !ok {
		return nil, &notFoundErr{id: id}
	}
	return c, nil
}

func (m *mockStore) CreateBackupRun(configID int64) (*store.BackupRun, error) {
	r := &store.BackupRun{ID: m.nextRunID, BackupConfigID: configID, Status: "running"}
	m.runs[m.nextRunID] = r
	m.nextRunID++
	return r, nil
}

func (m *mockStore) UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath, checksum string) error {
	m.successCall = &successArgs{id: id, sizeBytes: sizeBytes, filePath: filePath, checksum: checksum}
	if r, ok := m.runs[id]; ok {
		r.Status = "success"
	}
	return nil
}

func (m *mockStore) UpdateBackupRunFailed(id int64, errMsg string) error {
	m.failedCall = &failedArgs{id: id, errMsg: errMsg}
	if r, ok := m.runs[id]; ok {
		r.Status = "failed"
	}
	return nil
}

func (m *mockStore) ListOldBackupRuns(configID int64, keepCount int) ([]store.BackupRun, error) {
	return m.oldRuns, nil
}

func (m *mockStore) ListOldBackupRunsByTime(configID int64, maxAgeDays int) ([]store.BackupRun, error) {
	return m.oldRunsTime, nil
}

func (m *mockStore) DeleteBackupRun(id int64) error {
	m.deletedRuns = append(m.deletedRuns, id)
	delete(m.runs, id)
	return nil
}

func (m *mockStore) GetAppByID(id int64) (*store.App, error) {
	return &store.App{ID: id, Name: "testapp", Slug: "testapp"}, nil
}

func (m *mockStore) GetBackupRun(id int64) (*store.BackupRun, error) {
	r, ok := m.runs[id]
	if !ok {
		return nil, &notFoundErr{id: id}
	}
	return r, nil
}

type notFoundErr struct{ id int64 }

func (e *notFoundErr) Error() string { return "not found" }

// --- mock strategy ---

type mockStrategy struct {
	data     string
	filename string
	err      error
}

func (s *mockStrategy) Type() string { return "mock" }

func (s *mockStrategy) Detect(cfg *compose.AppConfig) []DetectedService { return nil }

func (s *mockStrategy) Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &BackupResult{
		Reader:   io.NopCloser(strings.NewReader(s.data)),
		Filename: s.filename,
	}, nil
}

func (s *mockStrategy) Restore(ctx context.Context, opts RestoreOpts) error {
	return s.err
}

// --- mock target ---

type mockTarget struct {
	uploaded  map[string][]byte
	err       error
	deleted   []string
	deleteErr map[string]error
}

func newMockTarget() *mockTarget {
	return &mockTarget{uploaded: make(map[string][]byte)}
}

func (t *mockTarget) Type() string { return "mock" }

func (t *mockTarget) Test(ctx context.Context) error { return nil }

func (t *mockTarget) Upload(ctx context.Context, filename string, data io.Reader) (string, int64, error) {
	if t.err != nil {
		return "", 0, t.err
	}
	b, _ := io.ReadAll(data)
	t.uploaded[filename] = b
	return filename, int64(len(b)), nil
}

func (t *mockTarget) Download(ctx context.Context, filename string) (io.ReadCloser, error) {
	b, ok := t.uploaded[filename]
	if !ok {
		return nil, &notFoundErr{}
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (t *mockTarget) Delete(ctx context.Context, filename string) error {
	t.deleted = append(t.deleted, filename)
	if err := t.deleteErr[filename]; err != nil {
		return err
	}
	delete(t.uploaded, filename)
	return nil
}

// --- tests ---

func TestSchedulerRunBackup(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{
		ID:             1,
		AppID:          10,
		Strategy:       "mock",
		Target:         "mock",
		RetentionMode:  "count",
		RetentionCount: 5,
	}

	tgt := newMockTarget()
	strategy := &mockStrategy{data: "backupdata", filename: "backup.tar.gz"}

	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", strategy)
	sched.RegisterTargetFactory("mock", func(configJSON string) (Target, error) {
		return tgt, nil
	})

	if err := sched.RunBackup(context.Background(), 1); err != nil {
		t.Fatalf("RunBackup error: %v", err)
	}

	if st.successCall == nil {
		t.Fatal("UpdateBackupRunSuccess not called")
	}
	if st.successCall.filePath != "backup.tar.gz" {
		t.Errorf("filePath = %q, want backup.tar.gz", st.successCall.filePath)
	}
	if st.successCall.sizeBytes != int64(len("backupdata")) {
		t.Errorf("sizeBytes = %d, want %d", st.successCall.sizeBytes, len("backupdata"))
	}
	if st.successCall.checksum == "" {
		t.Error("checksum should be set")
	}
	if _, ok := tgt.uploaded["backup.tar.gz"]; !ok {
		t.Error("backup.tar.gz not found in target uploads")
	}
	if st.failedCall != nil {
		t.Errorf("UpdateBackupRunFailed unexpectedly called: %v", st.failedCall.errMsg)
	}
}

func TestSchedulerRunBackupFailure(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{
		ID:       1,
		AppID:    10,
		Strategy: "mock",
		Target:   "mock",
	}

	strategy := &mockStrategy{err: &notFoundErr{}}
	tgt := newMockTarget()

	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", strategy)
	sched.RegisterTargetFactory("mock", func(configJSON string) (Target, error) {
		return tgt, nil
	})

	err := sched.RunBackup(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if st.failedCall == nil {
		t.Fatal("UpdateBackupRunFailed not called")
	}
	if !strings.Contains(st.failedCall.errMsg, "backup") {
		t.Errorf("errMsg = %q, expected to contain 'backup'", st.failedCall.errMsg)
	}
}

func TestScheduleAndUnschedule(t *testing.T) {
	st := newMockStore()
	sched := NewScheduler(st, nil)

	// Schedule a config
	err := sched.ScheduleConfig(1, "*/5 * * * *")
	if err != nil {
		t.Fatalf("ScheduleConfig: %v", err)
	}

	sched.mu.Lock()
	_, exists := sched.entries[1]
	sched.mu.Unlock()
	if !exists {
		t.Error("entry not found after ScheduleConfig")
	}

	// Reschedule (hot-reload)
	err = sched.ScheduleConfig(1, "0 * * * *")
	if err != nil {
		t.Fatalf("ScheduleConfig (reload): %v", err)
	}

	// Unschedule
	sched.UnscheduleConfig(1)

	sched.mu.Lock()
	_, exists = sched.entries[1]
	sched.mu.Unlock()
	if exists {
		t.Error("entry should be removed after UnscheduleConfig")
	}

	// Unschedule nonexistent (should not panic)
	sched.UnscheduleConfig(999)
}

func TestScheduleConfigInvalidCron(t *testing.T) {
	st := newMockStore()
	sched := NewScheduler(st, nil)

	err := sched.ScheduleConfig(1, "not a cron")
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestIsMissedBackupHourlyStale(t *testing.T) {
	// Hourly schedule, last run 3 hours ago -> missed (>2x 1h interval)
	threeHoursAgo := time.Now().Add(-3 * time.Hour)
	if !isMissedBackup("0 * * * *", &threeHoursAgo) {
		t.Error("expected missed for hourly schedule with 3h-old run")
	}
}

func TestIsMissedBackupDailyRecent(t *testing.T) {
	// Daily schedule, last run 12 hours ago -> not missed (<2x 24h interval)
	twelveHoursAgo := time.Now().Add(-12 * time.Hour)
	if isMissedBackup("0 0 * * *", &twelveHoursAgo) {
		t.Error("expected not missed for daily schedule with 12h-old run")
	}
}

func TestIsMissedBackupNilLastRun(t *testing.T) {
	if isMissedBackup("0 * * * *", nil) {
		t.Error("expected not missed when lastRun is nil")
	}
}

func TestGetDetector(t *testing.T) {
	st := newMockStore()
	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", &mockStrategy{})

	d := sched.GetDetector()
	if d == nil {
		t.Fatal("GetDetector returned nil")
	}
	if len(d.strategies) != 1 {
		t.Errorf("detector has %d strategies, want 1", len(d.strategies))
	}
}

func TestGetStrategy(t *testing.T) {
	st := newMockStore()
	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", &mockStrategy{})

	s, ok := sched.GetStrategy("mock")
	if !ok || s == nil {
		t.Error("GetStrategy(mock) should return registered strategy")
	}

	_, ok = sched.GetStrategy("nope")
	if ok {
		t.Error("GetStrategy(nope) should return false")
	}
}

func TestSetAlertFunc(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{
		ID:       1,
		AppID:    10,
		Strategy: "mock",
		Target:   "mock",
	}

	var alertCalled bool
	tgt := newMockTarget()
	strategy := &mockStrategy{data: "data", filename: "f.tar"}

	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("mock", strategy)
	sched.RegisterTargetFactory("mock", func(string) (Target, error) { return tgt, nil })
	sched.SetAlertFunc(func(appName, strategy, message, eventType string) {
		alertCalled = true
	})

	_ = sched.RunBackup(context.Background(), 1)
	if !alertCalled {
		t.Error("alert func should have been called on success")
	}
}

func TestParseHooks(t *testing.T) {
	hooks := parseHooks(`[{"type":"stop","service":"db"}]`)
	if len(hooks) != 1 {
		t.Fatalf("len = %d, want 1", len(hooks))
	}
	if hooks[0].Type != "stop" {
		t.Errorf("type = %q, want stop", hooks[0].Type)
	}

	// empty/null
	if len(parseHooks("")) != 0 {
		t.Error("empty string should return nil")
	}
	if len(parseHooks("null")) != 0 {
		t.Error("null should return nil")
	}
}

func TestParsePaths(t *testing.T) {
	paths := parsePaths(`["/data","/config"]`)
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2", len(paths))
	}

	// comma-separated fallback
	paths = parsePaths("/data,/config")
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2", len(paths))
	}
}

func TestParsePaths_TrimsCommaList(t *testing.T) {
	paths := parsePaths(" /data , /config ,")
	if len(paths) != 2 || paths[0] != "/data" || paths[1] != "/config" {
		t.Fatalf("parsePaths = %q, want [/data /config]", paths)
	}
}

func TestSchedulerRefusesUnsafePaths(t *testing.T) {
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "volume", Target: "mock", Paths: `["/data","/-rf"]`}
	st.runs[99] = &store.BackupRun{ID: 99, BackupConfigID: 1, Status: "success", FilePath: "x.tar.gz"}

	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("volume", NewVolumeStrategy())
	sched.RegisterTargetFactory("mock", func(string) (Target, error) { return newMockTarget(), nil })

	if err := sched.RunBackup(context.Background(), 1); err == nil {
		t.Fatal("RunBackup: expected error for unsafe path")
	}
	if st.failedCall == nil || !strings.Contains(st.failedCall.errMsg, "backup path") {
		t.Fatalf("expected failed run with path error, got %+v", st.failedCall)
	}
	if err := sched.RunRestore(context.Background(), 99); err == nil || !strings.Contains(err.Error(), "backup path") {
		t.Fatalf("RunRestore: expected path error, got %v", err)
	}
}

func TestSchedulerRunRestoreSizePolicy(t *testing.T) {
	spoolDir(t)
	st := newMockStore()
	st.configs[1] = &store.BackupConfig{ID: 1, AppID: 10, Strategy: "capture", Target: "mock"}
	tgt := newMockTarget()
	tgt.uploaded["b.tar.gz"] = []byte("payload")
	st.runs[7] = &store.BackupRun{ID: 7, BackupConfigID: 1, Status: "success", FilePath: "b.tar.gz", Checksum: checksumOf(t, "payload")}

	strategy := &readerCapture{}
	sched := NewScheduler(st, nil)
	sched.RegisterStrategy("capture", strategy)
	sched.RegisterTargetFactory("mock", func(string) (Target, error) { return tgt, nil })

	t.Setenv(RestoreMaxGBEnv, "")
	if err := sched.RunRestore(context.Background(), 7); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if strategy.maxBytes != NoDecompressedLimit {
		t.Errorf("default MaxDecompressedBytes = %d, want no limit", strategy.maxBytes)
	}
	if string(strategy.data) != "payload" {
		t.Errorf("restored %q, want payload", strategy.data)
	}

	t.Setenv(RestoreMaxGBEnv, "3")
	if err := sched.RunRestore(context.Background(), 7); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if strategy.maxBytes != 3<<30 {
		t.Errorf("MaxDecompressedBytes with %s=3 = %d, want %d", RestoreMaxGBEnv, strategy.maxBytes, int64(3<<30))
	}
}

func TestPruneOldRunsDeletesFilesAndRuns(t *testing.T) {
	for _, mode := range []string{"count", "time"} {
		t.Run(mode, func(t *testing.T) {
			st := newMockStore()
			days := 7
			cfg := &store.BackupConfig{ID: 1, AppID: 10, RetentionMode: mode, RetentionCount: 2, RetentionDays: &days}
			old := []store.BackupRun{
				{ID: 3, BackupConfigID: 1, Status: "success", FilePath: "old-3.tar.gz"},
				{ID: 2, BackupConfigID: 1, Status: "success", FilePath: "old-2.tar.gz"},
			}
			if mode == "count" {
				st.oldRuns = old
			} else {
				st.oldRunsTime = old
			}
			for i := range old {
				st.runs[old[i].ID] = &old[i]
			}
			tgt := newMockTarget()
			tgt.uploaded["old-3.tar.gz"] = []byte("x")
			tgt.uploaded["old-2.tar.gz"] = []byte("y")

			NewScheduler(st, nil).pruneOldRuns(context.Background(), cfg, tgt)

			if got := strings.Join(tgt.deleted, ","); got != "old-3.tar.gz,old-2.tar.gz" {
				t.Fatalf("target.Delete calls = %q", got)
			}
			if len(tgt.uploaded) != 0 {
				t.Errorf("files left on target: %v", tgt.uploaded)
			}
			// Pruned runs must no longer be offered for download/restore.
			if len(st.deletedRuns) != 2 || st.deletedRuns[0] != 3 || st.deletedRuns[1] != 2 {
				t.Fatalf("deleted runs = %v, want [3 2]", st.deletedRuns)
			}
			if len(st.runs) != 0 {
				t.Errorf("runs left = %v", st.runs)
			}
		})
	}
}

func TestPruneOldRunsKeepsRunWhenFileDeleteFails(t *testing.T) {
	st := newMockStore()
	st.oldRuns = []store.BackupRun{
		{ID: 5, BackupConfigID: 1, Status: "success", FilePath: "gone.tar.gz"},
		{ID: 4, BackupConfigID: 1, Status: "success", FilePath: "stuck.tar.gz"},
	}
	tgt := newMockTarget()
	tgt.deleteErr = map[string]error{
		// Already removed (e.g. by an older version that kept the row):
		// treat as pruned so it is not retried forever.
		"gone.tar.gz": &os.PathError{Op: "remove", Path: "gone.tar.gz", Err: os.ErrNotExist},
		// Real failure: keep the run so the next prune retries it.
		"stuck.tar.gz": errors.New("s3 unavailable"),
	}
	cfg := &store.BackupConfig{ID: 1, RetentionMode: "count", RetentionCount: 1}

	NewScheduler(st, nil).pruneOldRuns(context.Background(), cfg, tgt)

	if len(st.deletedRuns) != 1 || st.deletedRuns[0] != 5 {
		t.Fatalf("deleted runs = %v, want [5]", st.deletedRuns)
	}
}
