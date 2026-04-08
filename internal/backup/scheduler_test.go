package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

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
}

type successArgs struct {
	id        int64
	sizeBytes int64
	filePath  string
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

func (m *mockStore) UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath string) error {
	m.successCall = &successArgs{id: id, sizeBytes: sizeBytes, filePath: filePath}
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

func (s *mockStrategy) Backup(ctx context.Context, containerName string) (io.ReadCloser, string, error) {
	if s.err != nil {
		return nil, "", s.err
	}
	return io.NopCloser(strings.NewReader(s.data)), s.filename, nil
}

func (s *mockStrategy) Restore(ctx context.Context, containerName string, data io.Reader) error {
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

func (t *mockTarget) Upload(ctx context.Context, filename string, data io.Reader) (int64, error) {
	if t.err != nil {
		return 0, t.err
	}
	b, _ := io.ReadAll(data)
	t.uploaded[filename] = b
	return int64(len(b)), nil
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
		RetentionCount: 5,
	}

	tgt := newMockTarget()
	strategy := &mockStrategy{data: "backupdata", filename: "backup.tar.gz"}

	sched := NewScheduler(st)
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

	sched := NewScheduler(st)
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
