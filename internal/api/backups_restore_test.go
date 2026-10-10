package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/store"
)

// blockingStrategy's Restore signals started, then waits for release.
type blockingStrategy struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingStrategy) Type() string                                       { return "block" }
func (b *blockingStrategy) Detect(*compose.AppConfig) []backup.DetectedService { return nil }
func (b *blockingStrategy) Backup(context.Context, backup.BackupOpts) (*backup.BackupResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (b *blockingStrategy) Restore(context.Context, backup.RestoreOpts) error {
	b.started <- struct{}{}
	<-b.release
	return nil
}

// optsStrategy reports the RestoreOpts it was called with.
type optsStrategy struct{ got chan backup.RestoreOpts }

func (o *optsStrategy) Type() string                                       { return "opts" }
func (o *optsStrategy) Detect(*compose.AppConfig) []backup.DetectedService { return nil }
func (o *optsStrategy) Backup(context.Context, backup.BackupOpts) (*backup.BackupResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (o *optsStrategy) Restore(_ context.Context, opts backup.RestoreOpts) error {
	o.got <- opts
	return nil
}

// memTarget serves a fixed backup file.
type memTarget struct{}

func (memTarget) Type() string               { return "mem" }
func (memTarget) Test(context.Context) error { return nil }
func (memTarget) Upload(context.Context, string, io.Reader) (string, int64, error) {
	return "", 0, fmt.Errorf("not implemented")
}
func (memTarget) Download(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("backup")), nil
}
func (memTarget) Delete(context.Context, string) error { return nil }

func restoreRunRequest(t *testing.T, srv *Server, runID int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/backups/restore/%d", runID), nil)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestHandleRestore_SharesRestoreSlots(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: filepath.Join(t.TempDir(), "none.yml"), Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, err := st.GetAppBySlug("myapp")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &store.BackupConfig{AppID: app.ID, Strategy: "postgres", Target: "local", RetentionMode: "count", RetentionCount: 1}
	if err := st.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateBackupRun(cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateBackupRunSuccess(run.ID, 6, "f.tar.gz", ""); err != nil {
		t.Fatal(err)
	}

	strat := &blockingStrategy{started: make(chan struct{}, 4), release: make(chan struct{})}
	sched := backup.NewScheduler(st, nil)
	sched.RegisterStrategy("postgres", strat)
	sched.RegisterTargetFactory("local", func(string) (backup.Target, error) { return memTarget{}, nil })
	srv.SetBackupScheduler(sched)

	// Leave one slot free.
	for i := 0; i < cap(srv.restoreSem)-1; i++ {
		srv.restoreSem <- struct{}{}
	}
	if w := restoreRunRequest(t, srv, run.ID); w.Code != http.StatusAccepted {
		t.Fatalf("first restore: status = %d, body %q", w.Code, w.Body.String())
	}
	select {
	case <-strat.started:
	case <-time.After(5 * time.Second):
		t.Fatal("restore did not start")
	}

	// The running restore holds the last slot.
	w := restoreRunRequest(t, srv, run.ID)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("restore with no free slot: status = %d, want 429", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too many restores") {
		t.Errorf("429 body = %q, want a clear message", w.Body.String())
	}

	// The slot is released when the restore ends.
	close(strat.release)
	deadline := time.Now().Add(5 * time.Second)
	for len(srv.restoreSem) != cap(srv.restoreSem)-1 {
		if time.Now().After(deadline) {
			t.Fatalf("slot not released: %d of %d in use", len(srv.restoreSem), cap(srv.restoreSem))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w := restoreRunRequest(t, srv, run.ID); w.Code != http.StatusAccepted {
		t.Fatalf("restore after release: status = %d, body %q", w.Code, w.Body.String())
	}
}

func TestUploadRestore_TightensExistingTempDir(t *testing.T) {
	srv, rec := newUploadRestoreServer(t)
	tmp := filepath.Join(srv.dataDir, "tmp")
	// Older versions created the dir 0755.
	if err := os.Mkdir(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if w := uploadRestoreRequest(t, srv, "db"); w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	waitRestoreContainer(t, rec)
	fi, err := os.Stat(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("tmp dir perm = %o, want 700", perm)
	}
}

func TestUploadRestore_SizeCap(t *testing.T) {
	cases := map[string]int64{
		"":  8 << 30,
		"2": 2 << 30,
	}
	for env, want := range cases {
		t.Run("env="+env, func(t *testing.T) {
			t.Setenv(backup.RestoreMaxGBEnv, env)
			srv, _ := newUploadRestoreServer(t)
			strat := &optsStrategy{got: make(chan backup.RestoreOpts, 1)}
			srv.backupScheduler.RegisterStrategy("rec", strat)
			if w := uploadRestoreRequest(t, srv, "db"); w.Code != http.StatusAccepted {
				t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
			}
			select {
			case opts := <-strat.got:
				if opts.MaxDecompressedBytes != want {
					t.Fatalf("MaxDecompressedBytes = %d, want %d", opts.MaxDecompressedBytes, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("restore strategy was not called")
			}
		})
	}
}
