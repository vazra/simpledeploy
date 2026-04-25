package render

import (
	"strings"
	"testing"
)

func TestUnknownFallback(t *testing.T) {
	s, target := Render("totally", "unknown", nil, nil)
	if s != "totally: unknown" {
		t.Errorf("unexpected fallback: %q", s)
	}
	if target != "" {
		t.Errorf("expected empty target, got %q", target)
	}
}

func TestComposeImageChange(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	after := []byte(`{"services":{"web":{"image":"nginx:1.26"}}}`)
	s, target := Render("compose", "changed", before, after)
	if !strings.Contains(s, "nginx:1.25") || !strings.Contains(s, "nginx:1.26") {
		t.Errorf("expected before/after images in summary, got %q", s)
	}
	if target != "web" {
		t.Errorf("expected target=web, got %q", target)
	}
}

func TestComposeServiceAddedRemoved(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	after := []byte(`{"services":{"web":{"image":"nginx:1.25"},"redis":{"image":"redis:7"}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "redis added") {
		t.Errorf("expected redis added in summary, got %q", s)
	}

	before2 := []byte(`{"services":{"web":{"image":"nginx:1.25"},"old":{"image":"x"}}}`)
	after2 := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	s2, _ := Render("compose", "changed", before2, after2)
	if !strings.Contains(s2, "old removed") {
		t.Errorf("expected old removed in summary, got %q", s2)
	}
}

func TestComposeEnvDiff(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"2"}}}}`)
	after := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"3","C":"4"}}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "env B changed") {
		t.Errorf("missing env B changed: %q", s)
	}
	if !strings.Contains(s, "env C added") {
		t.Errorf("missing env C added: %q", s)
	}
}

func TestComposeReplicasChange(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","replicas":1}}}`)
	after := []byte(`{"services":{"web":{"image":"x","replicas":3}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "replicas 1 → 3") {
		t.Errorf("expected replicas diff in summary, got %q", s)
	}
}

func TestComposeEnvRemoved(t *testing.T) {
	before := []byte(`{"services":{"web":{"image":"x","env":{"A":"1","B":"2"}}}}`)
	after := []byte(`{"services":{"web":{"image":"x","env":{"A":"1"}}}}`)
	s, _ := Render("compose", "changed", before, after)
	if !strings.Contains(s, "env B removed") {
		t.Errorf("expected env B removed in summary, got %q", s)
	}
}
