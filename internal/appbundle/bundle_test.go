package appbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestBuildParseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", "services:\n  web:\n    image: nginx\n")
	writeFile(t, dir, "simpledeploy.yml", "display_name: My App\n")
	writeFile(t, dir, ".env", "# comment\n\nFOO=bar\nBAZ=\"quoted\"\nNOEQ\n")

	zipBytes, err := Build(dir, "myapp", "My App", "v1.2.3")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	b, err := Parse(zipBytes)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if b.Manifest.SchemaVersion != 1 {
		t.Errorf("schema_version: got %d", b.Manifest.SchemaVersion)
	}
	if b.Manifest.App.Slug != "myapp" {
		t.Errorf("slug: got %q", b.Manifest.App.Slug)
	}
	if b.Manifest.App.DisplayName != "My App" {
		t.Errorf("display_name: got %q", b.Manifest.App.DisplayName)
	}
	if b.Manifest.SourceSimpleDeployVersion != "v1.2.3" {
		t.Errorf("version: got %q", b.Manifest.SourceSimpleDeployVersion)
	}
	if b.Manifest.ExportedAt.IsZero() {
		t.Error("exported_at zero")
	}
	wantRedacted := []string{"env_values", "secrets"}
	if len(b.Manifest.Redacted) != len(wantRedacted) {
		t.Errorf("redacted: got %v", b.Manifest.Redacted)
	}
	if !bytes.Contains(b.Compose, []byte("nginx")) {
		t.Errorf("compose: %s", b.Compose)
	}
	if !bytes.Contains(b.Sidecar, []byte("My App")) {
		t.Errorf("sidecar: %s", b.Sidecar)
	}
	if b.EnvExample == nil {
		t.Fatal("env example nil")
	}
}

func TestBuildOnlyCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docker-compose.yml", "services: {}\n")

	zipBytes, err := Build(dir, "x", "", "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := Parse(zipBytes)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if b.Sidecar != nil {
		t.Errorf("sidecar should be nil, got %q", b.Sidecar)
	}
	if b.EnvExample != nil {
		t.Errorf("env should be nil, got %q", b.EnvExample)
	}
}

func TestEnvRedaction(t *testing.T) {
	input := []byte("# header comment\n\nFOO=bar\nBAZ=\"with spaces\"\nQUX='single'\nNOEQ\n# trailing\n")
	out := string(redactEnv(input))
	want := "# header comment\n\nFOO=\nBAZ=\nQUX=\nNOEQ\n# trailing\n"
	if out != want {
		t.Errorf("redactEnv:\n got: %q\nwant: %q", out, want)
	}
}

func TestEnvRedactionNoTrailingNewline(t *testing.T) {
	out := string(redactEnv([]byte("FOO=bar")))
	if out != "FOO=" {
		t.Errorf("got %q", out)
	}
}

func TestParseMissingManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("docker-compose.yml")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	if _, err := Parse(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Errorf("expected manifest error, got %v", err)
	}
}

func TestParseUnsupportedSchema(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	cw, _ := zw.Create("docker-compose.yml")
	_, _ = cw.Write([]byte("x"))
	mw, _ := zw.Create("manifest.json")
	m := Manifest{SchemaVersion: 99, App: AppMeta{Slug: "x"}}
	mb, _ := json.Marshal(m)
	_, _ = mw.Write(mb)
	_ = zw.Close()
	if _, err := Parse(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("expected schema error, got %v", err)
	}
}

func TestParseMissingCompose(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, _ := zw.Create("manifest.json")
	m := Manifest{SchemaVersion: 1, App: AppMeta{Slug: "x"}}
	mb, _ := json.Marshal(m)
	_, _ = mw.Write(mb)
	_ = zw.Close()
	if _, err := Parse(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "docker-compose") {
		t.Errorf("expected compose error, got %v", err)
	}
}

func TestParseZipSlip(t *testing.T) {
	cases := []string{"../evil.yml", "/etc/passwd", "foo/../bar"}
	for _, name := range cases {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte("x"))
		mw, _ := zw.Create("manifest.json")
		m := Manifest{SchemaVersion: 1, App: AppMeta{Slug: "x"}}
		mb, _ := json.Marshal(m)
		_, _ = mw.Write(mb)
		cw, _ := zw.Create("docker-compose.yml")
		_, _ = cw.Write([]byte("x"))
		_ = zw.Close()
		if _, err := Parse(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "unsafe") {
			t.Errorf("case %q: expected unsafe error, got %v", name, err)
		}
	}
}
