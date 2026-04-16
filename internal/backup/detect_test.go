package backup

import (
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
)

func TestDetector_DetectAll(t *testing.T) {
	d := NewDetector()
	d.Register(NewPostgresStrategy())
	d.Register(NewMySQLStrategy())
	d.Register(NewMongoStrategy())
	d.Register(NewRedisStrategy())
	d.Register(NewSQLiteStrategy())
	d.Register(NewVolumeStrategy())

	cfg := &compose.AppConfig{
		Name: "testapp",
		Services: []compose.ServiceConfig{
			{
				Name:  "db",
				Image: "postgres:16",
			},
			{
				Name:  "cache",
				Image: "redis:7-alpine",
			},
			{
				Name:  "web",
				Image: "nginx:latest",
				Volumes: []compose.VolumeMount{
					{Source: "html", Target: "/usr/share/nginx/html", Type: "volume"},
				},
			},
		},
	}

	results := d.DetectAll(cfg)
	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}

	// Build lookup by strategy type
	byType := map[string]DetectionResult{}
	for _, r := range results {
		byType[r.StrategyType] = r
	}

	// Postgres detected via image keyword
	pg := byType["postgres"]
	if !pg.Available {
		t.Error("postgres should be available")
	}
	if len(pg.Services) != 1 || pg.Services[0].ServiceName != "db" {
		t.Errorf("postgres services: %+v", pg.Services)
	}
	if pg.Label != "PostgreSQL" {
		t.Errorf("postgres label = %q", pg.Label)
	}

	// Redis detected via image keyword
	rd := byType["redis"]
	if !rd.Available {
		t.Error("redis should be available")
	}
	if len(rd.Services) != 1 || rd.Services[0].ServiceName != "cache" {
		t.Errorf("redis services: %+v", rd.Services)
	}

	// Volume detected from nginx volumes
	vol := byType["volume"]
	if !vol.Available {
		t.Error("volume should be available")
	}
	// All 3 services should have volume entries (pg data dir is implicit in image,
	// but volume strategy only detects explicit mounts, so just web)
	found := false
	for _, svc := range vol.Services {
		if svc.ServiceName == "web" {
			found = true
			if len(svc.Paths) != 1 || svc.Paths[0] != "/usr/share/nginx/html" {
				t.Errorf("web volume paths: %v", svc.Paths)
			}
		}
	}
	if !found {
		t.Error("volume should detect web service")
	}

	// MySQL, Mongo, SQLite should not be available
	for _, typ := range []string{"mysql", "mongo", "sqlite"} {
		r := byType[typ]
		if r.Available {
			t.Errorf("%s should not be available", typ)
		}
		if r.Description == "" {
			t.Errorf("%s should have unavailable description", typ)
		}
	}
}

func TestDetector_LabelOverride(t *testing.T) {
	d := NewDetector()
	d.Register(NewPostgresStrategy())
	d.Register(NewMySQLStrategy())

	cfg := &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{
			{
				Name:  "db",
				Image: "postgres:16",
				Labels: map[string]string{
					"simpledeploy.backup.strategy": "mysql",
				},
			},
		},
	}

	results := d.DetectAll(cfg)
	byType := map[string]DetectionResult{}
	for _, r := range results {
		byType[r.StrategyType] = r
	}

	// Label says mysql, so mysql strategy should pick it up
	mysql := byType["mysql"]
	if !mysql.Available {
		t.Error("mysql should detect label-overridden service")
	}

	// Postgres should still detect via image keyword
	pg := byType["postgres"]
	if !pg.Available {
		t.Error("postgres should still detect via image keyword")
	}
}

func TestDetector_EmptyConfig(t *testing.T) {
	d := NewDetector()
	d.Register(NewPostgresStrategy())

	cfg := &compose.AppConfig{Name: "empty"}
	results := d.DetectAll(cfg)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Available {
		t.Error("should not be available with no services")
	}
}
