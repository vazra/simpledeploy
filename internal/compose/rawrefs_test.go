package compose

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// parseViolations parses composeYAML as apps/<name>/docker-compose.yml and
// returns the ViolationError raised while parsing (nil when it loads).
func parseViolations(t *testing.T, appsDir, name, composeYAML string) []string {
	t.Helper()
	path := writeApp(t, appsDir, name, composeYAML, "")
	_, err := ParseFile(path, name)
	if err == nil {
		return nil
	}
	var ve *ViolationError
	if !errors.As(err, &ve) {
		t.Fatalf("ParseFile: want ViolationError, got %v", err)
	}
	return ve.Violations
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRawRefsRefuseUnsafeLabelFiles(t *testing.T) {
	_, appsDir := testApps(t)
	outside := filepath.Join(t.TempDir(), "labels")
	writeFile(t, outside, "a=b\n")
	appDir := filepath.Join(appsDir, "app")
	writeFile(t, filepath.Join(appDir, "ok.labels"), "a=b\n")
	if err := os.MkdirAll(filepath.Join(appDir, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(appDir, "link.labels")); err != nil {
		t.Fatal(err)
	}

	svc := "services:\n  web:\n    image: nginx\n"
	cases := map[string]string{
		"absolute":          svc + "    label_file: " + outside + "\n",
		"dotdot":            svc + "    label_file: ../app/ok.labels\n",
		"tilde":             svc + "    label_file: ~/labels\n",
		"variable":          svc + "    label_file: ${LABELS:-ok.labels}\n",
		"symlink outside":   svc + "    label_file: link.labels\n",
		"directory":         svc + "    label_file: dir\n",
		"missing":           svc + "    label_file: missing.labels\n",
		"list entry":        svc + "    label_file:\n      - ok.labels\n      - " + outside + "\n",
		"via merge key":     "x-base: &base\n  label_file: " + outside + "\n" + svc + "    <<: *base\n",
		"second document":   svc + "---\nservices:\n  web:\n    label_file: " + outside + "\n",
		"disabled profile":  svc + "  extra:\n    image: nginx\n    profiles: [off]\n    label_file: " + outside + "\n",
		"extended base svc": "services:\n  base:\n    image: nginx\n    label_file: " + outside + "\n  web:\n    extends: base\n",
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			wantViolation(t, parseViolations(t, appsDir, "app", c), "label_file")
		})
	}
}

func TestRawRefsAllowLabelFileInsideApp(t *testing.T) {
	_, appsDir := testApps(t)
	writeFile(t, filepath.Join(appsDir, "app", "conf", "web.labels"), "team=core\n")
	path := writeApp(t, appsDir, "app", "services:\n  web:\n    image: nginx\n    label_file: ./conf/web.labels\n", "")
	cfg, err := ParseFile(path, "app")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got := cfg.Project.Services["web"].Labels["team"]; got != "core" {
		t.Errorf("label team = %q, want core", got)
	}
}

func TestRawRefsRefuseUnsafeExtendsFiles(t *testing.T) {
	_, appsDir := testApps(t)
	base := "services:\n  base:\n    image: nginx\n"
	outside := filepath.Join(t.TempDir(), "base.yml")
	writeFile(t, outside, base)
	appDir := filepath.Join(appsDir, "app")
	writeFile(t, filepath.Join(appDir, "base.yml"), base)
	if err := os.Symlink(outside, filepath.Join(appDir, "link.yml")); err != nil {
		t.Fatal(err)
	}
	// An extends file inside the app folder that reaches outside again.
	writeFile(t, filepath.Join(appDir, "nested.yml"), "services:\n  base:\n    extends:\n      file: "+outside+"\n      service: base\n")
	writeFile(t, filepath.Join(appDir, "sub", "labels.yml"), "services:\n  base:\n    image: nginx\n    label_file: ../../../etc/labels\n")

	ext := func(file string) string {
		return "services:\n  web:\n    extends:\n      file: " + file + "\n      service: base\n"
	}
	cases := map[string]struct{ compose, want string }{
		"absolute":          {ext(outside), "extends file"},
		"dotdot":            {ext("../app/base.yml"), "extends file"},
		"tilde":             {ext("~/base.yml"), "extends file"},
		"variable":          {ext("${BASE}"), "extends file"},
		"symlink outside":   {ext("link.yml"), "extends file"},
		"directory":         {ext("."), "extends file"},
		"nested outside":    {ext("nested.yml"), "extends file"},
		"label in extended": {ext("sub/labels.yml"), "label_file"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantViolation(t, parseViolations(t, appsDir, "app", tc.compose), tc.want)
		})
	}
}

func TestRawRefsAllowSameFileAndLocalExtends(t *testing.T) {
	_, appsDir := testApps(t)
	writeFile(t, filepath.Join(appsDir, "app", "common", "base.yml"), "services:\n  base:\n    image: nginx\n    environment:\n      FROM: file\n")
	cases := map[string]string{
		"string form":  "services:\n  base:\n    image: nginx\n  web:\n    extends: base\n",
		"mapping form": "services:\n  base:\n    image: nginx\n  web:\n    extends:\n      service: base\n",
		"local file":   "services:\n  web:\n    extends:\n      file: common/base.yml\n      service: base\n",
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeApp(t, appsDir, "app", c, "")
			cfg, err := ParseFile(path, "app")
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if img := cfg.Project.Services["web"].Image; img != "nginx" {
				t.Errorf("web image = %q, want nginx", img)
			}
		})
	}
}

func TestRawRefsRefuseIncludeBeforeLoading(t *testing.T) {
	_, appsDir := testApps(t)
	// A missing include target would make the loader fail with a read
	// error; the raw check must refuse it first.
	for name, c := range map[string]string{
		"first document":  "include:\n  - /nonexistent/extra.yml\nservices:\n  web:\n    image: nginx\n",
		"second document": "services:\n  web:\n    image: nginx\n---\ninclude:\n  - /nonexistent/extra.yml\n",
	} {
		t.Run(name, func(t *testing.T) {
			wantViolation(t, parseViolations(t, appsDir, "app", c), `"include" is not supported`)
		})
	}
}

func TestRawRefsUnknownAppFolder(t *testing.T) {
	// A compose file checked for a folder that does not exist yet (a new
	// app) cannot reference files.
	path := filepath.Join(t.TempDir(), "missing", "docker-compose.yml")
	_, err := ParseContent([]byte("services:\n  web:\n    image: nginx\n    label_file: labels\n"), path, "app", nil)
	var ve *ViolationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Error(), "label_file") {
		t.Fatalf("want label_file violation, got %v", err)
	}
	if _, err := ParseContent([]byte("services:\n  web:\n    image: nginx\n"), path, "app", nil); err != nil {
		t.Fatalf("plain compose: %v", err)
	}
}

// ParseForRoutes keeps the endpoints of a file refused over its file
// references, without reading those files or the .env.
func TestParseForRoutesDropsFileRefs(t *testing.T) {
	_, appsDir := testApps(t)
	outside := filepath.Join(t.TempDir(), "labels")
	writeFile(t, outside, "simpledeploy.endpoints.0.domain=evil.example.com\n")
	c := "include:\n  - /nonexistent/extra.yml\n" +
		"x-base: &base\n  label_file: " + outside + "\n" +
		"services:\n  base:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: ${DOMAIN:-app.example.com}\n      simpledeploy.endpoints.0.port: \"80\"\n" +
		"  web:\n    <<: *base\n    extends: base\n" +
		"  api:\n    image: nginx\n    label_file: [" + outside + "]\n    extends:\n      file: " + outside + "\n      service: x\n"
	path := writeApp(t, appsDir, "app", c, "DOMAIN=fromenv.example.com\n")
	var ve *ViolationError
	if _, err := ParseFile(path, "app"); !errors.As(err, &ve) {
		t.Fatalf("ParseFile: want ViolationError, got %v", err)
	}
	cfg, err := ParseForRoutes(path, "app")
	if err != nil {
		t.Fatalf("ParseForRoutes: %v", err)
	}
	services := map[string]bool{}
	for _, ep := range cfg.Endpoints {
		if ep.Domain != "app.example.com" {
			t.Errorf("endpoint %+v, want app.example.com (no label_file, no .env)", ep)
		}
		services[ep.Service] = true
	}
	if !services["base"] || !services["web"] {
		t.Errorf("endpoints %+v, want base and web (same-file extends kept)", cfg.Endpoints)
	}
	if _, ok := cfg.Project.Services["api"]; !ok {
		t.Error("service api dropped")
	}
	if cfg.ComposePath != path {
		t.Errorf("ComposePath = %q, want %q", cfg.ComposePath, path)
	}
}

func TestParseForRoutesRefusesSymlinkedCompose(t *testing.T) {
	_, appsDir := testApps(t)
	target := filepath.Join(t.TempDir(), "compose.yml")
	writeFile(t, target, "services:\n  web:\n    image: nginx\n")
	dir := filepath.Join(appsDir, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "docker-compose.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseForRoutes(link, "app"); err == nil {
		t.Fatal("ParseForRoutes read a symlinked compose file")
	}
}

func TestRawRefsCoverJobsAndPreStartHooks(t *testing.T) {
	_, appsDir := testApps(t)
	outside := filepath.Join(t.TempDir(), "base.yml")
	writeFile(t, outside, "services:\n  base:\n    image: busybox\n")
	job := "services:\n  web:\n    image: nginx\njobs:\n  task:\n    image: busybox\n    triggers:\n      manual: true\n"
	cases := map[string]struct{ compose, want string }{
		"job extends file": {job + "    extends:\n      file: " + outside + "\n      service: base\n", `job "task": extends file`},
		"job label_file":   {job + "    label_file: " + outside + "\n", `job "task": label_file`},
		"hook label_file": {"services:\n  web:\n    image: nginx\n    pre_start:\n      - command: [\"true\"]\n        label_file: " + outside + "\n",
			`service "web" pre_start hook 1: label_file`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantViolation(t, parseViolations(t, appsDir, "app", tc.compose), tc.want)
		})
	}

	// A job may extend a service from a file inside the app folder.
	writeFile(t, filepath.Join(appsDir, "app", "common", "base.yml"), "services:\n  base:\n    image: alpine\n")
	path := writeApp(t, appsDir, "app", "services:\n  web:\n    image: nginx\njobs:\n  task:\n    triggers:\n      manual: true\n    extends:\n      file: common/base.yml\n      service: base\n", "")
	cfg, err := ParseFile(path, "app")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if img := cfg.Project.Jobs["task"].Image; img != "alpine" {
		t.Errorf("job image = %q, want alpine", img)
	}
}

func TestParseForRoutesDropsJobAndHookFileRefs(t *testing.T) {
	_, appsDir := testApps(t)
	outside := filepath.Join(t.TempDir(), "base.yml")
	writeFile(t, outside, "services:\n  base:\n    image: busybox\n")
	c := "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: app.example.com\n" +
		"    pre_start:\n      - label_file: " + outside + "\n" +
		"jobs:\n  task:\n    image: busybox\n    triggers:\n      manual: true\n    label_file: " + outside + "\n" +
		"    extends:\n      file: " + outside + "\n      service: base\n"
	path := writeApp(t, appsDir, "app", c, "")
	var ve *ViolationError
	if _, err := ParseFile(path, "app"); !errors.As(err, &ve) {
		t.Fatalf("ParseFile: want ViolationError, got %v", err)
	}
	cfg, err := ParseForRoutes(path, "app")
	if err != nil {
		t.Fatalf("ParseForRoutes: %v", err)
	}
	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0].Domain != "app.example.com" {
		t.Errorf("endpoints = %+v, want app.example.com", cfg.Endpoints)
	}
	if _, ok := cfg.Project.Jobs["task"]; !ok {
		t.Error("job task dropped")
	}
}
