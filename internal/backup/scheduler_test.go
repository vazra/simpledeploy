package backup

import (
	"bytes"
	"context"
	"io"
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
	uploaded map[string][]byte
	err      error
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
