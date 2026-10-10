package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/store"
)

// S3 target_config_json comes in two spellings: snake_case keys from the UI
// wizard and docs, and Go field names (AccessKey, ...) from older clients
// and configs stored before S3Config had json tags. Both must reach the S3
// client on every path: create/update (encrypted at rest), test-s3, and the
// pre-signed download.
var s3ConfigSpellings = []struct {
	name string
	json func(endpoint, accessKey, secretKey string) string
}{
	{"snake_case", func(endpoint, ak, sk string) string {
		b, _ := json.Marshal(map[string]string{"endpoint": endpoint, "bucket": "bkt", "prefix": "pre", "access_key": ak, "secret_key": sk, "region": "eu-west-1"})
		return string(b)
	}},
	{"legacy", func(endpoint, ak, sk string) string {
		b, _ := json.Marshal(map[string]string{"Endpoint": endpoint, "Bucket": "bkt", "Prefix": "pre", "AccessKey": ak, "SecretKey": sk, "Region": "eu-west-1"})
		return string(b)
	}},
}

func TestTestS3_ReadsCredentialsInBothSpellings(t *testing.T) {
	t.Setenv(backup.AllowPrivateS3Env, "1")
	for _, sp := range s3ConfigSpellings {
		t.Run(sp.name, func(t *testing.T) {
			var mu sync.Mutex
			var authz []string
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				authz = append(authz, r.Header.Get("Authorization"))
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer fake.Close()

			var body map[string]string
			_ = json.Unmarshal([]byte(sp.json(fake.URL, "AKIDTEST"+sp.name, "secret")), &body)
			srv, _ := newTestServer(t)
			w := postTestS3(t, srv, body)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
				t.Fatalf("status = %d body = %s, want ok:true", w.Code, w.Body.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if len(authz) == 0 {
				t.Fatal("fake S3 endpoint was not contacted")
			}
			want := "Credential=AKIDTEST" + sp.name + "/"
			if !strings.Contains(authz[0], want) {
				t.Errorf("Authorization %q does not contain %q", authz[0], want)
			}
			if !strings.Contains(authz[0], "/eu-west-1/s3/") {
				t.Errorf("Authorization %q does not use region eu-west-1", authz[0])
			}
		})
	}
}

// The UI relies on this contract: connection failures come back as 200 with
// {"ok": false, "error": "..."}; only invalid input is a 4xx.
func TestTestS3_FailureReportsOkFalse(t *testing.T) {
	t.Setenv(backup.AllowPrivateS3Env, "1")
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>InvalidAccessKeyId</Code><Message>bad key</Message></Error>`))
	}))
	defer fake.Close()

	srv, _ := newTestServer(t)
	w := postTestS3(t, srv, map[string]string{"endpoint": fake.URL, "bucket": "b", "access_key": "k", "secret_key": "s"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body: %s", err, w.Body.String())
	}
	if resp.OK || resp.Error == "" {
		t.Fatalf("got %+v, want ok=false with an error message", resp)
	}
	if !strings.Contains(resp.Error, "InvalidAccessKeyId") {
		t.Errorf("error %q should carry the S3 error code", resp.Error)
	}
}

func TestBackupConfig_S3CredentialsSurviveEncryption(t *testing.T) {
	for _, sp := range s3ConfigSpellings {
		t.Run(sp.name, func(t *testing.T) {
			srv, st := newTestServer(t)
			srv.masterSecret = "test-master-secret"
			st.UpsertApp(&store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/myapp.yml", Status: "running"}, nil)

			body := baseBackupConfig()
			body["target"] = "s3"
			body["target_config_json"] = sp.json("", "AKIDSTORED", "stored-secret")
			w := postBackupConfig(t, srv, body)
			if w.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
			}
			var created store.BackupConfig
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatalf("decode: %v", err)
			}
			stored, err := st.GetBackupConfig(created.ID)
			if err != nil {
				t.Fatalf("GetBackupConfig: %v", err)
			}
			if strings.Contains(stored.TargetConfigJSON, "stored-secret") {
				t.Fatal("S3 secret stored in plaintext")
			}
			plain, err := auth.Decrypt(stored.TargetConfigJSON, srv.masterSecret)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			var cfg backup.S3Config
			if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			want := backup.S3Config{Bucket: "bkt", Prefix: "pre", AccessKey: "AKIDSTORED", SecretKey: "stored-secret", Region: "eu-west-1"}
			if cfg != want {
				t.Errorf("decrypted config = %+v, want %+v", cfg, want)
			}
		})
	}
}

func TestDownloadBackup_S3PresignUsesStoredCredentials(t *testing.T) {
	for _, sp := range s3ConfigSpellings {
		t.Run(sp.name, func(t *testing.T) {
			srv, st := newTestServer(t)
			srv.masterSecret = "test-master-secret"
			app := &store.App{Name: "myapp", Slug: "myapp", ComposePath: "/tmp/myapp.yml", Status: "running"}
			if err := st.UpsertApp(app, nil); err != nil {
				t.Fatal(err)
			}
			app, err := st.GetAppBySlug("myapp")
			if err != nil {
				t.Fatal(err)
			}
			enc, err := auth.Encrypt(sp.json("", "AKIDPRESIGN", "presign-secret"), srv.masterSecret)
			if err != nil {
				t.Fatal(err)
			}
			cfg := &store.BackupConfig{AppID: app.ID, Strategy: "volume", Target: "s3", TargetConfigJSON: enc, RetentionMode: "count", RetentionCount: 3}
			if err := st.CreateBackupConfig(cfg); err != nil {
				t.Fatal(err)
			}
			run, err := st.CreateBackupRun(cfg.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.UpdateBackupRunSuccess(run.ID, 10, "pre/myapp.tar.gz", "abc"); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/backups/runs/"+strconv.FormatInt(run.ID, 10)+"/download", nil)
			req.AddCookie(superAdminCookie(t, srv.jwt))
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("status = %d, want 307; body: %s", w.Code, w.Body.String())
			}
			loc, err := url.Parse(w.Header().Get("Location"))
			if err != nil {
				t.Fatalf("parse Location: %v", err)
			}
			cred := loc.Query().Get("X-Amz-Credential")
			if !strings.HasPrefix(cred, "AKIDPRESIGN/") || !strings.Contains(cred, "/eu-west-1/s3/") {
				t.Errorf("X-Amz-Credential = %q, want access key AKIDPRESIGN in eu-west-1", cred)
			}
			if !strings.Contains(loc.Host+loc.Path, "bkt") {
				t.Errorf("presigned URL %s does not reference bucket bkt", loc)
			}
		})
	}
}
