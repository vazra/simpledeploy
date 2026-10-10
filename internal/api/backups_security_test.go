package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/store"
)

// recordingStrategy is a backup.Strategy whose Restore reports the
// container it was asked to restore into.
type recordingStrategy struct{ got chan string }

func (r *recordingStrategy) Type() string                                       { return "rec" }
func (r *recordingStrategy) Detect(*compose.AppConfig) []backup.DetectedService { return nil }
func (r *recordingStrategy) Backup(context.Context, backup.BackupOpts) (*backup.BackupResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *recordingStrategy) Restore(_ context.Context, opts backup.RestoreOpts) error {
	r.got <- opts.ContainerName
	return nil
}

// namedMockDocker wraps docker.MockClient (whose AddContainer only sets ID
// and labels) to also report container names.
type namedMockDocker struct {
	*docker.MockClient
	names map[string]string // id -> name
}

func (d *namedMockDocker) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	list, err := d.MockClient.ContainerList(ctx, opts)
	for i := range list {
		if n, ok := d.names[list[i].ID]; ok {
			list[i].Names = []string{"/" + n}
		}
	}
	return list, err
}

const (
	ownDBID     = "aaaaaaaaaaaa0000000000000000000000000000000000000000000000000001"
	ownWebID    = "aaaaaaaaaaab0000000000000000000000000000000000000000000000000002"
	foreignDBID = "bbbbbbbbbbbb0000000000000000000000000000000000000000000000000003"
)

func newUploadRestoreServer(t *testing.T) (*Server, *recordingStrategy) {
	t.Helper()
	srv, st := newTestServer(t)
	if err := st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/myapp.yml", Status: "running"}, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	srv.SetDataDir(t.TempDir())

	rec := &recordingStrategy{got: make(chan string, 1)}
	sched := backup.NewScheduler(st, nil)
	sched.RegisterStrategy("rec", rec)
	srv.SetBackupScheduler(sched)

	mock := docker.NewMockClient()
	mock.AddContainer(ownDBID, map[string]string{
		"com.docker.compose.project": "simpledeploy-myapp",
		"com.docker.compose.service": "db",
	})
	mock.AddContainer(ownWebID, map[string]string{
		"com.docker.compose.project": "simpledeploy-myapp",
		"com.docker.compose.service": "web",
	})
	mock.AddContainer(foreignDBID, map[string]string{
		"com.docker.compose.project": "simpledeploy-other",
		"com.docker.compose.service": "db",
	})
	srv.SetDocker(&namedMockDocker{MockClient: mock, names: map[string]string{
		ownDBID:     "simpledeploy-myapp-db-1",
		ownWebID:    "simpledeploy-myapp-web-1",
		foreignDBID: "simpledeploy-other-db-1",
	}})
	return srv, rec
}

func uploadRestoreRequest(t *testing.T, srv *Server, containerField string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("strategy", "rec")
	if containerField != "" {
		mw.WriteField("container", containerField)
	}
	fw, _ := mw.CreateFormFile("file", "dump.sql")
	fw.Write([]byte("SELECT 1;"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/backups/upload-restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func waitRestoreContainer(t *testing.T, rec *recordingStrategy) string {
	t.Helper()
	select {
	case got := <-rec.got:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("restore strategy was not called")
		return ""
	}
}

func TestUploadRestore_OwnContainerAccepted(t *testing.T) {
	cases := map[string]string{
		"name":          "simpledeploy-myapp-db-1",
		"name with /":   "/simpledeploy-myapp-db-1",
		"full id":       ownDBID,
		"id prefix":     ownDBID[:12],
		"service name":  "db",
		"other service": "web",
	}
	want := map[string]string{"other service": "simpledeploy-myapp-web-1"}
	for name, field := range cases {
		t.Run(name, func(t *testing.T) {
			srv, rec := newUploadRestoreServer(t)
			w := uploadRestoreRequest(t, srv, field)
			if w.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
			}
			exp := want[name]
			if exp == "" {
				exp = "simpledeploy-myapp-db-1"
			}
			if got := waitRestoreContainer(t, rec); got != exp {
				t.Fatalf("restored into %q, want %q", got, exp)
			}
		})
	}
}

func TestUploadRestore_ForeignContainerRejected(t *testing.T) {
	for _, field := range []string{
		"simpledeploy-other-db-1",
		"/simpledeploy-other-db-1",
		foreignDBID,
		foreignDBID[:12],
		ownDBID[:8], // ID prefix too short to be unambiguous
		"some-random-container",
	} {
		t.Run(field, func(t *testing.T) {
			srv, rec := newUploadRestoreServer(t)
			w := uploadRestoreRequest(t, srv, field)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "not part of app") {
				t.Errorf("body should explain the rejection: %s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "simpledeploy-other") && !strings.Contains(field, "simpledeploy-other") {
				t.Errorf("error leaks another app's container: %s", w.Body.String())
			}
			select {
			case got := <-rec.got:
				t.Fatalf("restore ran against %q", got)
			default:
			}
		})
	}
}

func TestUploadRestore_EmptyContainerResolved(t *testing.T) {
	// No container named or serviced "myapp": ask the user to choose.
	srv, _ := newUploadRestoreServer(t)
	w := uploadRestoreRequest(t, srv, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "db") || !strings.Contains(w.Body.String(), "web") {
		t.Errorf("body should list the app's services: %s", w.Body.String())
	}

	// A service named like the app resolves through the same check.
	srv, rec := newUploadRestoreServer(t)
	nd := srv.docker.(*namedMockDocker)
	nd.AddContainer("cccccccccccc", map[string]string{
		"com.docker.compose.project": "simpledeploy-myapp",
		"com.docker.compose.service": "myapp",
	})
	nd.names["cccccccccccc"] = "simpledeploy-myapp-myapp-1"
	w = uploadRestoreRequest(t, srv, "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
	if got := waitRestoreContainer(t, rec); got != "simpledeploy-myapp-myapp-1" {
		t.Fatalf("restored into %q, want simpledeploy-myapp-myapp-1", got)
	}
}

func TestUploadRestore_NoDocker(t *testing.T) {
	srv, rec := newUploadRestoreServer(t)
	srv.docker = nil

	w := uploadRestoreRequest(t, srv, "db")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("non-empty container without docker: status = %d, want 400", w.Code)
	}

	// Empty keeps the historical app-name default.
	w = uploadRestoreRequest(t, srv, "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("empty container without docker: status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
	if got := waitRestoreContainer(t, rec); got != "myapp" {
		t.Fatalf("restored into %q, want myapp", got)
	}
}

func TestUploadRestore_SemaphoreBeforeTempFile(t *testing.T) {
	srv, _ := newUploadRestoreServer(t)
	for i := 0; i < cap(srv.restoreSem); i++ {
		srv.restoreSem <- struct{}{}
	}
	w := uploadRestoreRequest(t, srv, "db")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	// The slot is checked before anything touches disk, so tmp is never
	// even created for a rejected upload.
	if _, err := os.Stat(filepath.Join(srv.dataDir, "tmp")); !os.IsNotExist(err) {
		t.Fatalf("rejected upload touched the tmp dir (stat err = %v)", err)
	}
}

func TestUploadRestore_TempDirOwnerOnly(t *testing.T) {
	srv, rec := newUploadRestoreServer(t)
	w := uploadRestoreRequest(t, srv, "db")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}
	waitRestoreContainer(t, rec)
	fi, err := os.Stat(filepath.Join(srv.dataDir, "tmp"))
	if err != nil {
		t.Fatalf("stat tmp: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("tmp dir perm = %o, want 700", perm)
	}
}

// --- backup config validation ---

func postBackupConfig(t *testing.T, srv *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/myapp/backups/configs", bytes.NewReader(b))
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func baseBackupConfig() map[string]any {
	return map[string]any{
		"strategy":        "volume",
		"target":          "local",
		"schedule_cron":   "0 2 * * *",
		"retention_mode":  "count",
		"retention_count": 3,
	}
}

func TestBackupConfig_Validation(t *testing.T) {
	t.Setenv(backup.AllowPrivateS3Env, "")
	cases := []struct {
		name     string
		override map[string]any
		wantCode int
		wantMsg  string
	}{
		{"valid", nil, http.StatusCreated, ""},
		{"valid paths", map[string]any{"paths": `["/data","/srv/files"]`}, http.StatusCreated, ""},
		{"no schedule", map[string]any{"schedule_cron": ""}, http.StatusCreated, ""},
		{"bad cron", map[string]any{"schedule_cron": "every day"}, http.StatusBadRequest, "invalid backup schedule"},
		{"six-field cron", map[string]any{"schedule_cron": "0 0 2 * * *"}, http.StatusBadRequest, "invalid backup schedule"},
		{"relative path", map[string]any{"paths": `["data"]`}, http.StatusBadRequest, "absolute"},
		{"flag path", map[string]any{"paths": `["/data/--checkpoint-action=exec=sh"]`}, http.StatusBadRequest, "'-'"},
		{"newline path", map[string]any{"paths": "[\"/data\\n/etc\"]"}, http.StatusBadRequest, "control"},
		{"sqlite unsafe chars", map[string]any{"strategy": "sqlite", "paths": `["/data/app'.sqlite"]`}, http.StatusBadRequest, "SQLite"},
		{"sqlite ok", map[string]any{"strategy": "sqlite", "paths": `["/data/app.db"]`}, http.StatusCreated, ""},
		{"sqlite space ok", map[string]any{"strategy": "sqlite", "paths": `["/data/my app@1.db"]`}, http.StatusCreated, ""},
		{"s3 private endpoint", map[string]any{"target": "s3", "target_config_json": `{"Endpoint":"http://127.0.0.1:9000","Bucket":"b"}`}, http.StatusBadRequest, backup.AllowPrivateS3Env},
		{"s3 metadata endpoint", map[string]any{"target": "s3", "target_config_json": `{"Endpoint":"http://169.254.169.254","Bucket":"b"}`}, http.StatusBadRequest, backup.AllowPrivateS3Env},
		{"s3 aws default", map[string]any{"target": "s3", "target_config_json": `{"Endpoint":"","Bucket":"b"}`}, http.StatusCreated, ""},
		{"s3 bad json", map[string]any{"target": "s3", "target_config_json": `{"Endpoint":"http://10.0.0.1","Bucket":1}`}, http.StatusBadRequest, "invalid S3 settings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newTestServer(t)
			st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/myapp.yml", Status: "running"}, nil)
			body := baseBackupConfig()
			for k, v := range tc.override {
				body[k] = v
			}
			w := postBackupConfig(t, srv, body)
			if w.Code != tc.wantCode {
				t.Fatalf("create status = %d, want %d; body: %s", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.wantMsg != "" && !strings.Contains(w.Body.String(), tc.wantMsg) {
				t.Errorf("create body %q should contain %q", w.Body.String(), tc.wantMsg)
			}

			// Same rules on update.
			cfgID := seedBackupConfigForApp(t, st, "upd")
			b, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/backups/configs/%d", cfgID), bytes.NewReader(b))
			req.AddCookie(superAdminCookie(t, srv.jwt))
			uw := httptest.NewRecorder()
			srv.Handler().ServeHTTP(uw, req)
			wantUpd := tc.wantCode
			if wantUpd == http.StatusCreated {
				wantUpd = http.StatusOK
			}
			if uw.Code != wantUpd {
				t.Fatalf("update status = %d, want %d; body: %s", uw.Code, wantUpd, uw.Body.String())
			}
		})
	}
}

func TestBackupConfig_PrivateS3OptIn(t *testing.T) {
	t.Setenv(backup.AllowPrivateS3Env, "1")
	srv, st := newTestServer(t)
	st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/myapp.yml", Status: "running"}, nil)
	body := baseBackupConfig()
	body["target"] = "s3"
	body["target_config_json"] = `{"Endpoint":"http://127.0.0.1:9000","Bucket":"b"}`
	if w := postBackupConfig(t, srv, body); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 with opt-in; body: %s", w.Code, w.Body.String())
	}
}

// --- test-s3 ---

func postTestS3(t *testing.T, srv *Server, cfg map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/backups/test-s3", bytes.NewReader(b))
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func TestTestS3_PrivateEndpoint(t *testing.T) {
	var hits atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer fake.Close()
	cfg := map[string]string{"Endpoint": fake.URL, "Bucket": "b", "AccessKey": "k", "SecretKey": "s"}

	t.Run("blocked", func(t *testing.T) {
		t.Setenv(backup.AllowPrivateS3Env, "")
		srv, _ := newTestServer(t)
		w := postTestS3(t, srv, cfg)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), backup.AllowPrivateS3Env) {
			t.Errorf("body should mention %s: %s", backup.AllowPrivateS3Env, w.Body.String())
		}
		if hits.Load() != 0 {
			t.Fatalf("blocked endpoint received %d requests", hits.Load())
		}
	})

	t.Run("opt-in", func(t *testing.T) {
		t.Setenv(backup.AllowPrivateS3Env, "1")
		srv, _ := newTestServer(t)
		w := postTestS3(t, srv, cfg)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
			t.Fatalf("status = %d body = %s, want ok:true", w.Code, w.Body.String())
		}
		if hits.Load() == 0 {
			t.Fatal("expected the local S3 endpoint to be contacted")
		}
	})
}

// --- compose version restore ---

func TestRestoreComposeVersion_DoesNotFollowSymlink(t *testing.T) {
	srv, st := newTestServer(t)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.Symlink(outside, composePath); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertApp(&store.App{Name: "symapp", Slug: "symapp", ComposePath: composePath, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	app, _ := st.GetAppBySlug("symapp")
	content := "services:\n  web:\n    image: nginx:2\n"
	if err := st.CreateComposeVersion(app.ID, content, "sha256:x"); err != nil {
		t.Fatal(err)
	}
	versions, _ := st.ListComposeVersions(app.ID)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/apps/symapp/versions/%d/restore", versions[0].ID), nil)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", w.Code, w.Body.String())
	}

	if got, _ := os.ReadFile(outside); string(got) != "original" {
		t.Fatalf("symlink target was overwritten: %q", got)
	}
	fi, err := os.Lstat(composePath)
	if err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("compose path should be a regular file now: %v %v", fi, err)
	}
	if got, _ := os.ReadFile(composePath); !strings.Contains(string(got), "nginx:2") {
		t.Fatalf("compose content not restored: %q", got)
	}
}

// Display names must not break the compose project lookup (projects are
// named after the slug).
func TestUploadRestore_DisplayNamedApp(t *testing.T) {
	srv, rec := newUploadRestoreServer(t)
	if err := srv.store.UpdateAppDisplayName("myapp", "My App"); err != nil {
		t.Fatal(err)
	}
	w := uploadRestoreRequest(t, srv, "db")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if got := waitRestoreContainer(t, rec); got != "simpledeploy-myapp-db-1" {
		t.Fatalf("container = %q", got)
	}
}

// An app with a single container needs no container choice.
func TestUploadRestore_SingleContainerDefault(t *testing.T) {
	srv, st := newTestServer(t)
	if err := st.UpsertApp(&store.App{Name: "solo", Slug: "solo", ComposePath: "/tmp/solo.yml", Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	srv.SetDataDir(t.TempDir())
	rec := &recordingStrategy{got: make(chan string, 1)}
	sched := backup.NewScheduler(st, nil)
	sched.RegisterStrategy("rec", rec)
	srv.SetBackupScheduler(sched)
	mock := docker.NewMockClient()
	mock.AddContainer(ownDBID, map[string]string{
		"com.docker.compose.project": "simpledeploy-solo",
		"com.docker.compose.service": "db",
	})
	srv.SetDocker(&namedMockDocker{MockClient: mock, names: map[string]string{ownDBID: "simpledeploy-solo-db-1"}})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("strategy", "rec")
	fw, _ := mw.CreateFormFile("file", "dump.sql")
	fw.Write([]byte("SELECT 1;"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/apps/solo/backups/upload-restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %q", w.Code, w.Body.String())
	}
	if got := waitRestoreContainer(t, rec); got != "simpledeploy-solo-db-1" {
		t.Fatalf("container = %q", got)
	}
}

// Detected container names use the app's slug (the compose project), not
// its display name.
func TestDetectStrategiesUsesSlug(t *testing.T) {
	srv, appsDir := newDeployTestServer(t)
	path := writeAppFile(t, appsDir, "myapp", "docker-compose.yml", "services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/data\n")
	if err := srv.store.UpsertApp(&store.App{Name: "My App", Slug: "myapp", ComposePath: path, Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	sched := backup.NewScheduler(srv.store, nil)
	sched.RegisterStrategy("volume", backup.NewVolumeStrategy())
	srv.SetBackupScheduler(sched)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/backups/detect", nil)
	req.AddCookie(superAdminCookie(t, srv.jwt))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	var resp struct {
		Strategies []backup.DetectionResult `json:"strategies"`
		Error      string                   `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Strategies) != 1 || len(resp.Strategies[0].Services) != 1 ||
		resp.Strategies[0].Services[0].ContainerName != "simpledeploy-myapp-web-1" {
		t.Fatalf("resp = %+v, want container simpledeploy-myapp-web-1", resp)
	}
}
