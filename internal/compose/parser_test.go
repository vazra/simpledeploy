package compose

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestParseBasicCompose(t *testing.T) {
	cfg, err := ParseFile(testdataPath("basic.yml"), "basicapp")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if cfg.Name != "basicapp" {
		t.Errorf("Name = %q, want %q", cfg.Name, "basicapp")
	}
	if cfg.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", cfg.Domain, "example.com")
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.TLS != "letsencrypt" {
		t.Errorf("TLS = %q, want %q", cfg.TLS, "letsencrypt")
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("len(Services) = %d, want 1", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Image != "nginx:alpine" {
		t.Errorf("Image = %q, want %q", svc.Image, "nginx:alpine")
	}
	if svc.Name != "web" {
		t.Errorf("service Name = %q, want %q", svc.Name, "web")
	}
	if cfg.Project == nil {
		t.Error("Project is nil")
	}
}

func TestParseMultiServiceCompose(t *testing.T) {
	cfg, err := ParseFile(testdataPath("multi.yml"), "multiapp")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(cfg.Services) != 2 {
		t.Fatalf("len(Services) = %d, want 2", len(cfg.Services))
	}

	// find db service
	var dbSvc *ServiceConfig
	for i := range cfg.Services {
		if cfg.Services[i].Name == "db" {
			dbSvc = &cfg.Services[i]
			break
		}
	}
	if dbSvc == nil {
		t.Fatal("db service not found")
	}

	if cfg.BackupStrategy != "dump" {
		t.Errorf("BackupStrategy = %q, want %q", cfg.BackupStrategy, "dump")
	}
	if cfg.BackupSchedule != "0 2 * * *" {
		t.Errorf("BackupSchedule = %q, want %q", cfg.BackupSchedule, "0 2 * * *")
	}
	if cfg.BackupTarget != "s3://mybucket/backups" {
		t.Errorf("BackupTarget = %q, want %q", cfg.BackupTarget, "s3://mybucket/backups")
	}
	if cfg.BackupRetention != "7d" {
		t.Errorf("BackupRetention = %q, want %q", cfg.BackupRetention, "7d")
	}

	if dbSvc.Image != "postgres:15" {
		t.Errorf("db Image = %q, want %q", dbSvc.Image, "postgres:15")
	}
	if len(dbSvc.Volumes) != 1 {
		t.Errorf("db Volumes len = %d, want 1", len(dbSvc.Volumes))
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/compose.yml", "app")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestExtractLabels(t *testing.T) {
	labels := map[string]string{
		"simpledeploy.domain":           "test.example.com",
		"simpledeploy.port":             "9090",
		"simpledeploy.tls":              "letsencrypt",
		"simpledeploy.backup.strategy":  "dump",
		"simpledeploy.backup.schedule":  "0 3 * * *",
		"simpledeploy.backup.target":    "s3://bucket/path",
		"simpledeploy.backup.retention": "14d",
		"simpledeploy.alert.cpu":        "80",
		"simpledeploy.alert.memory":     "90",
		"simpledeploy.paths":            "/api,/web",
		"simpledeploy.ratelimit.requests": "100",
		"simpledeploy.ratelimit.window":   "1m",
		"simpledeploy.ratelimit.by":       "ip",
		"simpledeploy.ratelimit.burst":    "20",
		"simpledeploy.access.allow":       "10.0.0.0/8,192.168.1.5",
	}

	lc := ExtractLabels(labels)

	if lc.Domain != "test.example.com" {
		t.Errorf("Domain = %q, want %q", lc.Domain, "test.example.com")
	}
	if lc.Port != "9090" {
		t.Errorf("Port = %q, want %q", lc.Port, "9090")
	}
	if lc.TLS != "letsencrypt" {
		t.Errorf("TLS = %q, want %q", lc.TLS, "letsencrypt")
	}
	if lc.BackupStrategy != "dump" {
		t.Errorf("BackupStrategy = %q, want %q", lc.BackupStrategy, "dump")
	}
	if lc.BackupSchedule != "0 3 * * *" {
		t.Errorf("BackupSchedule = %q, want %q", lc.BackupSchedule, "0 3 * * *")
	}
	if lc.BackupTarget != "s3://bucket/path" {
		t.Errorf("BackupTarget = %q, want %q", lc.BackupTarget, "s3://bucket/path")
	}
	if lc.BackupRetention != "14d" {
		t.Errorf("BackupRetention = %q, want %q", lc.BackupRetention, "14d")
	}
	if lc.AlertCPU != "80" {
		t.Errorf("AlertCPU = %q, want %q", lc.AlertCPU, "80")
	}
	if lc.AlertMemory != "90" {
		t.Errorf("AlertMemory = %q, want %q", lc.AlertMemory, "90")
	}
	if lc.PathPatterns != "/api,/web" {
		t.Errorf("PathPatterns = %q, want %q", lc.PathPatterns, "/api,/web")
	}
	if lc.RateLimit.Requests != "100" {
		t.Errorf("RateLimit.Requests = %q, want %q", lc.RateLimit.Requests, "100")
	}
	if lc.RateLimit.Window != "1m" {
		t.Errorf("RateLimit.Window = %q, want %q", lc.RateLimit.Window, "1m")
	}
	if lc.RateLimit.By != "ip" {
		t.Errorf("RateLimit.By = %q, want %q", lc.RateLimit.By, "ip")
	}
	if lc.RateLimit.Burst != "20" {
		t.Errorf("RateLimit.Burst = %q, want %q", lc.RateLimit.Burst, "20")
	}
	if lc.AccessAllow != "10.0.0.0/8,192.168.1.5" {
		t.Errorf("AccessAllow = %q, want %q", lc.AccessAllow, "10.0.0.0/8,192.168.1.5")
	}
}
