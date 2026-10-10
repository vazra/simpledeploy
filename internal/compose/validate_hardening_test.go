package compose

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

// testApps creates data and apps folders under a temp root and registers
// them as protected for the duration of the test.
func testApps(t *testing.T) (dataDir, appsDir string) {
	t.Helper()
	root := t.TempDir()
	dataDir = filepath.Join(root, "data")
	appsDir = filepath.Join(root, "apps")
	for _, d := range []string{dataDir, appsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	SetProtectedPaths(dataDir, appsDir)
	t.Cleanup(func() { SetProtectedPaths("", "") })
	return dataDir, appsDir
}

// writeApp writes apps/<name>/docker-compose.yml (and .env when dotEnv is
// non-empty) and returns the compose path.
func writeApp(t *testing.T, appsDir, name, composeYAML, dotEnv string) string {
	t.Helper()
	dir := filepath.Join(appsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(composeYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if dotEnv != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotEnv), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func validateApp(t *testing.T, appsDir, name, composeYAML, dotEnv string) []string {
	t.Helper()
	path := writeApp(t, appsDir, name, composeYAML, dotEnv)
	cfg, err := ParseFile(path, name)
	// File references the loader would read are refused while parsing.
	var ve *ViolationError
	if errors.As(err, &ve) {
		return ve.Violations
	}
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return ValidateComposeSecurity(cfg)
}

func wantViolation(t *testing.T, v []string, substr string) {
	t.Helper()
	for _, m := range v {
		if strings.Contains(m, substr) {
			return
		}
	}
	t.Fatalf("want a violation containing %q, got %v", substr, v)
}

func wantNone(t *testing.T, v []string) {
	t.Helper()
	if len(v) != 0 {
		t.Fatalf("want no violations, got %v", v)
	}
}

func TestParseInterpolatesDotEnv(t *testing.T) {
	_, appsDir := testApps(t)
	compose := "services:\n  web:\n    image: nginx\n    privileged: ${PRIV}\n    volumes:\n      - ${SRC}:/host\n"
	v := validateApp(t, appsDir, "envapp", compose, "PRIV=true\nSRC=/etc\n")
	wantViolation(t, v, "privileged")
	wantViolation(t, v, `"/etc"`)
}

func TestParseProcessEnvOverridesDotEnv(t *testing.T) {
	_, appsDir := testApps(t)
	t.Setenv("SD_TEST_BIND_SRC", "/etc")
	compose := "services:\n  web:\n    image: nginx\n    volumes:\n      - ${SD_TEST_BIND_SRC}:/host\n"
	v := validateApp(t, appsDir, "envapp", compose, "SD_TEST_BIND_SRC=./data\n")
	wantViolation(t, v, `"/etc"`)
}

func TestParseRefusesSymlinkedComposeAndDotEnv(t *testing.T) {
	_, appsDir := testApps(t)
	outside := filepath.Join(t.TempDir(), "other.yml")
	if err := os.WriteFile(outside, []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(appsDir, "linked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "docker-compose.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(filepath.Join(dir, "docker-compose.yml"), "linked"); err == nil {
		t.Fatal("want error for symlinked compose file")
	}

	path := writeApp(t, appsDir, "envlink", "services:\n  web:\n    image: nginx\n", "")
	if err := os.Symlink(outside, filepath.Join(appsDir, "envlink", ".env")); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(path, "envlink"); !errors.Is(err, ErrDotEnv) {
		t.Fatalf("err = %v, want ErrDotEnv for symlinked .env", err)
	}
}

func TestParseRejectsBrokenDotEnv(t *testing.T) {
	_, appsDir := testApps(t)
	path := writeApp(t, appsDir, "badenv", "services:\n  web:\n    image: nginx\n", "PASS=\"unterminated\n")
	if _, err := ParseFile(path, "badenv"); !errors.Is(err, ErrDotEnv) {
		t.Fatalf("err = %v, want ErrDotEnv", err)
	}
}

func TestParseContentResolvesAgainstTargetPath(t *testing.T) {
	_, appsDir := testApps(t)
	content := []byte("services:\n  web:\n    image: nginx\n    privileged: ${PRIV}\n    volumes:\n      - ./data:/data\n      - ../other/data:/other\n")
	cfg, err := ParseContent(content, filepath.Join(appsDir, "newapp", "docker-compose.yml"), "newapp", []byte("PRIV=false\n"))
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	v := ValidateComposeSecurity(cfg)
	if len(v) != 1 {
		t.Fatalf("want exactly one violation, got %v", v)
	}
	wantViolation(t, v, "another app's folder")
}

func TestValidateDisabledProfileServices(t *testing.T) {
	_, appsDir := testApps(t)
	compose := "services:\n  web:\n    image: nginx\n  debug:\n    image: alpine\n    profiles: [debug]\n    privileged: true\n"
	v := validateApp(t, appsDir, "profapp", compose, "")
	wantViolation(t, v, `service "debug": privileged`)
}

func TestValidateBindRules(t *testing.T) {
	dataDir, appsDir := testApps(t)
	writeApp(t, appsDir, "other", "services:\n  web:\n    image: nginx\n", "")
	cases := []struct {
		name, src, want string
	}{
		{"own folder", "./data", ""},
		{"own folder root writable", ".", "app folder itself"},
		{"other app", "../other", "another app's folder"},
		{"apps dir", "..", "every app's folder"},
		{"data dir", dataDir, "data folder"},
		{"inside data dir", filepath.Join(dataDir, "simpledeploy.db"), "data folder"},
		{"parent of data dir", filepath.Dir(dataDir), "data folder"},
		{"root", "/", "is not allowed"},
		{"var", "/var", "is not allowed"},
		{"var lib", "/var/lib/mysql", "protected system folder"},
		{"docker sock", "/var/run/docker.sock", "protected system folder"},
		{"home", "/home/user/.ssh", "protected system folder"},
		{"usr", "/usr/local/bin", "protected system folder"},
		{"srv allowed", "/srv/media", ""},
		{"opt allowed", "/opt/data", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compose := "services:\n  web:\n    image: nginx\n    volumes:\n      - type: bind\n        source: " + tc.src + "\n        target: /x\n"
			v := validateApp(t, appsDir, "app", compose, "")
			if tc.want == "" {
				wantNone(t, v)
				return
			}
			wantViolation(t, v, tc.want)
		})
	}
}

func TestValidateBindNormalizesPaths(t *testing.T) {
	for _, src := range []string{"//etc", "/./etc", "/srv/../etc/", "/var//run/docker.sock"} {
		t.Run(src, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{
				Name:    "x",
				Volumes: []types.ServiceVolumeConfig{{Type: "bind", Source: src, Target: "/x"}},
			}))
			wantViolation(t, v, "protected system folder")
		})
	}
}

func TestValidateBindFollowsSymlinks(t *testing.T) {
	dataDir, appsDir := testApps(t)
	otherData := filepath.Join(appsDir, "other", "data")
	if err := os.MkdirAll(otherData, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(appsDir, "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dataDir, filepath.Join(dir, "db")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherData, filepath.Join(dir, "shared")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"./db":       "links to",
		"./db/x":     "links to",
		"./shared":   "links to",
		"./dangling": "could not be checked",
	}
	for src, want := range cases {
		t.Run(src, func(t *testing.T) {
			compose := "services:\n  web:\n    image: nginx\n    volumes:\n      - " + src + ":/x\n"
			v := validateApp(t, appsDir, "app", compose, "")
			wantViolation(t, v, want)
		})
	}
	wantViolation(t, validateApp(t, appsDir, "app", "services:\n  web:\n    image: nginx\n    volumes:\n      - ./db:/x\n", ""), "data folder")
	wantViolation(t, validateApp(t, appsDir, "app", "services:\n  web:\n    image: nginx\n    volumes:\n      - ./shared:/x\n", ""), "another app's folder")

	// A plain subfolder stays fine.
	v := validateApp(t, appsDir, "app", "services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/x\n", "")
	wantNone(t, v)
}

func TestValidateFileReferencesConfinedToAppFolder(t *testing.T) {
	_, appsDir := testApps(t)
	// label_file must exist for the loader, so point it at a real file
	// outside the app folder.
	outsideFile := filepath.Join(t.TempDir(), "labels")
	if err := os.WriteFile(outsideFile, []byte("a=b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, compose, want string
	}{
		{"secret outside", "services:\n  web:\n    image: nginx\n    secrets: [s]\nsecrets:\n  s:\n    file: /etc/shadow\n", `secret "s"`},
		{"config outside", "services:\n  web:\n    image: nginx\n    configs: [c]\nconfigs:\n  c:\n    file: ../other/conf\n", `config "c"`},
		{"env_file outside", "services:\n  web:\n    image: nginx\n    env_file: /etc/environment\n", "env_file"},
		{"label_file outside", "services:\n  web:\n    image: nginx\n    label_file: " + outsideFile + "\n", "label_file"},
		{"build context outside", "services:\n  web:\n    build: /\n", "build context"},
		{"dockerfile outside", "services:\n  web:\n    build:\n      context: .\n      dockerfile: /etc/Dockerfile\n", "dockerfile"},
		{"dockerfile escapes context", "services:\n  web:\n    build:\n      context: ./src\n      dockerfile: ../../other/Dockerfile\n", "dockerfile"},
		{"additional context outside", "services:\n  web:\n    build:\n      context: .\n      additional_contexts:\n        host: /etc\n", "build context"},
		{"build ssh key outside", "services:\n  web:\n    build:\n      context: .\n      ssh:\n        - key=/root/.ssh/id_rsa\n", "ssh key"},
		{"build network host", "services:\n  web:\n    build:\n      context: .\n      network: host\n", "build network"},
		{"privileged build", "services:\n  web:\n    build:\n      context: .\n      privileged: true\n", "privileged build"},
		{"build entitlement", "services:\n  web:\n    build:\n      context: .\n      entitlements: [security.insecure]\n", "entitlement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validateApp(t, appsDir, "app", tc.compose, "")
			wantViolation(t, v, tc.want)
		})
	}
}

func TestValidateFileReferencesInsideAppFolderAllowed(t *testing.T) {
	_, appsDir := testApps(t)
	compose := `services:
  web:
    build:
      context: .
      dockerfile: Dockerfile
      additional_contexts:
        base: docker-image://alpine:3
        assets: ./assets
    env_file: .env
    secrets: [s]
    configs: [c]
  remote:
    build: https://github.com/example/repo.git
secrets:
  s:
    file: ./secrets/s.txt
configs:
  c:
    file: ./conf/app.conf
`
	v := validateApp(t, appsDir, "app", compose, "A=1\n")
	wantNone(t, v)
}

func TestValidateIncludeAndExtends(t *testing.T) {
	_, appsDir := testApps(t)
	other := writeApp(t, appsDir, "other", "services:\n  base:\n    image: nginx\n", "")

	inc := writeApp(t, appsDir, "inc", "include:\n  - ./extra.yml\nservices:\n  web:\n    image: nginx\n", "")
	if err := os.WriteFile(filepath.Join(filepath.Dir(inc), "extra.yml"), []byte("services:\n  db:\n    image: redis\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ParseFile(inc, "inc")
	var ve *ViolationError
	if !errors.As(err, &ve) {
		t.Fatalf("ParseFile: want ViolationError, got %v", err)
	}
	wantViolation(t, ve.Violations, `"include" is not supported`)

	v := validateApp(t, appsDir, "ext", "services:\n  web:\n    extends:\n      file: "+other+"\n      service: base\n", "")
	wantViolation(t, v, "extends file")

	local := writeApp(t, appsDir, "extok", "services:\n  web:\n    extends:\n      file: base.yml\n      service: base\n", "")
	if err := os.WriteFile(filepath.Join(filepath.Dir(local), "base.yml"), []byte("services:\n  base:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseFile(local, "extok")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	wantNone(t, ValidateComposeSecurity(cfg))
}

func TestValidateVolumeDriverOpts(t *testing.T) {
	_, appsDir := testApps(t)
	vol := func(body string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - v:/data\nvolumes:\n  v:\n" + body
	}
	cases := []struct {
		name, compose, want string
	}{
		{"block device", vol("    driver_opts:\n      type: ext4\n      device: /dev/sda1\n"), "type \"ext4\""},
		{"overlay", vol("    driver_opts:\n      type: overlay\n      device: overlay\n      o: lowerdir=/etc\n"), "type \"overlay\""},
		{"proc", vol("    driver_opts:\n      type: proc\n      device: proc\n"), "type \"proc\""},
		{"relative bind device", vol("    driver_opts:\n      type: none\n      o: bind,ro\n      device: data\n"), "must be a full path"},
		{"bind shim rbind", vol("    driver_opts:\n      type: none\n      o: rbind,ro\n      device: /var/lib/docker\n"), "protected system folder"},
		{"third-party driver", vol("    driver: local-persist\n    driver_opts:\n      mountpoint: /etc\n"), "protected system folder"},
		{"other app volume", vol("    name: simpledeploy-other_data\n"), "from another app"},
		{"other app external", vol("    external: true\n    name: simpledeploy-other_data\n"), "from another app"},
		{"own prefix other key", vol("    name: simpledeploy-app_data\n"), `only simpledeploy- name allowed is the default "simpledeploy-app_v"`},
		{"own prefix longer key", vol("    name: simpledeploy-app_v_x\n"), `only simpledeploy- name allowed is the default "simpledeploy-app_v"`},
		{"external key of other app", "services:\n  web:\n    image: nginx\n    volumes:\n      - simpledeploy-other_data:/data\nvolumes:\n  simpledeploy-other_data:\n    external: true\n", "from another app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validateApp(t, appsDir, "app", tc.compose, "")
			wantViolation(t, v, tc.want)
		})
	}

	allowed := []string{
		vol("    driver_opts:\n      type: nfs\n      o: addr=10.0.0.5,rw\n      device: \":/export/data\"\n"),
		vol("    driver_opts:\n      type: cifs\n      device: //nas/share\n"),
		vol("    driver_opts:\n      type: tmpfs\n      device: tmpfs\n      o: size=100m\n"),
		vol("    driver: local\n    driver_opts:\n      type: none\n      o: bind\n      device: ./data\n"),
		vol("    name: simpledeploy-app_v\n"),
		vol("    external: true\n    name: shared-media\n"),
	}
	for i, c := range allowed {
		if v := validateApp(t, appsDir, "app", c, ""); len(v) != 0 {
			t.Errorf("allowed case %d: got %v", i, v)
		}
	}
}

// Only a name with another app's prefix is reported as that app's storage.
func TestValidateVolumeNameMessage(t *testing.T) {
	_, appsDir := testApps(t)
	vol := func(name string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - v:/data\nvolumes:\n  v:\n    name: " + name + "\n"
	}
	for _, name := range []string{"simpledeploy-app_data", "simpledeploy-shared"} {
		v := validateApp(t, appsDir, "app", vol(name), "")
		wantViolation(t, v, `the only simpledeploy- name allowed is the default "simpledeploy-app_v"`)
		for _, m := range v {
			if strings.Contains(m, "another app") {
				t.Errorf("%s: message claims another app: %q", name, m)
			}
		}
	}
	v := validateApp(t, appsDir, "app", vol("simpledeploy-other_data"), "")
	wantViolation(t, v, `uses storage "simpledeploy-other_data" from another app; only the default name "simpledeploy-app_v" is allowed`)
}

// Local volumes that bind a host folder follow the same layout rules as
// bind mounts.
func TestValidateVolumeDeviceBindLayout(t *testing.T) {
	// The key is not a single letter: "v:/x" reads as a Windows drive path.
	_, appsDir := testApps(t)
	devVol := func(device, o string) string {
		return "volumes:\n  store:\n    driver_opts:\n      type: none\n      o: " + o + "\n      device: " + device + "\n"
	}
	appFolder := func(app, mount, o string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - " + mount + "\n" + devVol(filepath.Join(appsDir, app), o)
	}
	wantViolation(t, validateApp(t, appsDir, "devapp", appFolder("devapp", "store:/app", "bind"), ""), "writable bind of the app folder itself")
	wantNone(t, validateApp(t, appsDir, "devapp-ro", appFolder("devapp-ro", "store:/app:ro", "bind"), ""))
	wantNone(t, validateApp(t, appsDir, "devapp-optro", appFolder("devapp-optro", "store:/app", "bind,ro"), ""))

	// Device inside a writable ./data bind.
	nested := func(app, mount string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/data\n  worker:\n    image: nginx\n    volumes:\n      - " + mount + "\n" +
			devVol(filepath.Join(appsDir, app, "data", "sub"), "bind")
	}
	wantViolation(t, validateApp(t, appsDir, "devnested", nested("devnested", "store:/sub"), ""), "inside the writable bind")

	// Device as the outer folder: refused when writable, fine when read-only.
	outer := func(app, mount, o string) string {
		return "services:\n  web:\n    image: nginx\n    volumes:\n      - " + mount + "\n  worker:\n    image: nginx\n    volumes:\n      - ./data/sub:/sub\n" +
			devVol(filepath.Join(appsDir, app, "data"), o)
	}
	wantViolation(t, validateApp(t, appsDir, "devouter", outer("devouter", "store:/data", "bind"), ""), "inside the writable bind")
	wantNone(t, validateApp(t, appsDir, "devouter-ro", outer("devouter-ro", "store:/data:ro", "bind"), ""))
	wantNone(t, validateApp(t, appsDir, "devouter-optro", outer("devouter-optro", "store:/data", "rbind,ro"), ""))
}

func TestValidateCapabilityNormalization(t *testing.T) {
	for _, c := range []string{"Cap_Sys_Admin", "cap_sys_admin", "sys_admin", " SYS_ADMIN ", "cap_all"} {
		t.Run(c, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", CapAdd: []string{c}}))
			wantViolation(t, v, "dangerous capability")
		})
	}
}

func TestValidateSecurityOptForms(t *testing.T) {
	bad := []string{
		"seccomp:unconfined",
		"seccomp=/opt/profiles/allow-all.json",
		"apparmor:unconfined",
		"label:disable",
		"label=type:spc_t",
		"systempaths:unconfined",
		"no-new-privileges:false",
	}
	for _, opt := range bad {
		t.Run(opt, func(t *testing.T) {
			v := ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", SecurityOpt: []string{opt}}))
			wantViolation(t, v, "security_opt")
		})
	}
	ok := []string{"no-new-privileges:true", "no-new-privileges", "label=level:s0:c100,c200", "apparmor=docker-default", "seccomp=builtin"}
	for _, opt := range ok {
		t.Run(opt, func(t *testing.T) {
			wantNone(t, ValidateComposeSecurity(cfgWith(types.ServiceConfig{Name: "x", SecurityOpt: []string{opt}})))
		})
	}
}

func TestValidateNamespaceSharing(t *testing.T) {
	bad := []types.ServiceConfig{
		{Name: "x", NetworkMode: "container:other"},
		{Name: "x", Ipc: "container:other"},
		{Name: "x", Uts: "host"},
		{Name: "x", DeviceCgroupRules: []string{"c 1:3 mr"}},
	}
	for _, svc := range bad {
		if v := ValidateComposeSecurity(cfgWith(svc)); len(v) == 0 {
			t.Errorf("want violation for %+v", svc)
		}
	}
	ok := []types.ServiceConfig{
		{Name: "x", NetworkMode: "service:db"},
		{Name: "x", NetworkMode: "bridge"},
		{Name: "x", Ipc: "shareable"},
		{Name: "x", Ipc: "private"},
	}
	for _, svc := range ok {
		wantNone(t, ValidateComposeSecurity(cfgWith(svc)))
	}
}

func TestValidateNilProject(t *testing.T) {
	if v := ValidateComposeSecurity(&AppConfig{}); len(v) == 0 {
		t.Fatal("want violation for config without a project")
	}
	if v := ValidateComposeSecurity(nil); len(v) == 0 {
		t.Fatal("want violation for nil config")
	}
}

func TestValidateReadOnlyVarLog(t *testing.T) {
	_, appsDir := testApps(t)
	wantNone(t, validateApp(t, appsDir, "logs-ro",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /var/log:/host/log:ro\n", ""))
	wantViolation(t, validateApp(t, appsDir, "logs-rw",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /var/log:/host/log\n", ""), "/var/log")
	// Read-only does not relax other system folders.
	wantViolation(t, validateApp(t, appsDir, "etc-ro",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /etc:/host/etc:ro\n", ""), "/etc")
	wantViolation(t, validateApp(t, appsDir, "var-ro",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /var:/host/var:ro\n", ""), "/var")
}

func TestValidateOperatorAllowedHostPaths(t *testing.T) {
	dataDir, appsDir := testApps(t)
	SetAllowedHostPaths([]string{"/home/media", "relative/ignored", "/"})
	t.Cleanup(func() { SetAllowedHostPaths(nil) })

	wantNone(t, validateApp(t, appsDir, "media",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /home/media/movies:/movies\n", ""))
	// Other system folders stay refused; "/" was ignored.
	wantViolation(t, validateApp(t, appsDir, "other",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - /home/someone:/x\n", ""), "/home/someone")

	// The data folder stays protected even when allow-listed.
	SetAllowedHostPaths([]string{dataDir})
	wantViolation(t, validateApp(t, appsDir, "data",
		"services:\n  a:\n    image: nginx\n    volumes:\n      - "+dataDir+":/x\n", ""), "data folder")
}

func TestValidateAppFolderReadOnlyAndNestedBinds(t *testing.T) {
	_, appsDir := testApps(t)
	wantNone(t, validateApp(t, appsDir, "ro", "services:\n  web:\n    image: nginx\n    volumes:\n      - .:/app:ro\n", ""))
	wantNone(t, validateApp(t, appsDir, "siblings", "services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/data\n      - ./logs:/logs\n", ""))
	wantViolation(t, validateApp(t, appsDir, "nested",
		"services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/data\n  worker:\n    image: nginx\n    volumes:\n      - ./data/sub:/sub\n", ""), "inside the writable bind")
	// A read-only outer bind cannot be rearranged, so nesting under it is fine.
	wantNone(t, validateApp(t, appsDir, "nested-ro",
		"services:\n  web:\n    image: nginx\n    volumes:\n      - ./data:/data:ro\n      - ./data/sub:/sub\n", ""))
}

func TestValidateRejectsHostReachingServiceOptions(t *testing.T) {
	_, appsDir := testApps(t)
	cases := map[string]struct{ yaml, want string }{
		"api socket":        {"services:\n  web:\n    image: nginx\n    use_api_socket: true\n", "use_api_socket"},
		"privileged hook":   {"services:\n  web:\n    image: nginx\n    post_start:\n      - command: [\"true\"]\n        privileged: true\n", "hooks not allowed"},
		"provider":          {"services:\n  web:\n    provider:\n      type: example\n", "provider services"},
		"host network name": {"services:\n  web:\n    image: nginx\nnetworks:\n  default:\n    name: host\n    external: true\n", "host or container networking"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantViolation(t, validateApp(t, appsDir, "opt", tc.yaml, ""), tc.want)
		})
	}
	wantNone(t, validateApp(t, appsDir, "hook-ok", "services:\n  web:\n    image: nginx\n    post_start:\n      - command: [\"true\"]\n", ""))
}
