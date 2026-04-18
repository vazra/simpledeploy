package compose

import (
	"bytes"
	"strings"
	"testing"
)

const networkName = "simpledeploy-public"

func TestInjectSharedNetworkBasic(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: example.com
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	s := string(out)
	if !strings.Contains(s, "networks:") {
		t.Error("missing top-level networks block")
	}
	if !strings.Contains(s, "simpledeploy-public:") {
		t.Error("missing simpledeploy-public under networks")
	}
	if !strings.Contains(s, "external: true") {
		t.Error("missing external: true")
	}
	// Service web should now have a networks list containing simpledeploy-public.
	if !strings.Contains(s, "- simpledeploy-public") {
		t.Errorf("web service not attached to simpledeploy-public:\n%s", s)
	}
}

func TestInjectSharedNetworkIdempotent(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: example.com
`)
	once, changed1, err := InjectSharedNetwork(src, networkName)
	if err != nil || !changed1 {
		t.Fatalf("first run: changed=%v err=%v", changed1, err)
	}
	twice, changed2, err := InjectSharedNetwork(once, networkName)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if changed2 {
		t.Error("expected changed=false on second run")
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("second run mutated bytes:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestInjectSharedNetworkNoEndpointUntouched(t *testing.T) {
	src := []byte(`services:
  db:
    image: postgres
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected changed=false when no endpoint services exist")
	}
	if !bytes.Equal(out, src) {
		t.Errorf("expected unchanged bytes:\n--- got ---\n%s\n--- want ---\n%s", out, src)
	}
}

func TestInjectSharedNetworkShortForm(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: example.com
    networks:
      - default
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	s := string(out)
	if !strings.Contains(s, "- default") {
		t.Errorf("should preserve 'default':\n%s", s)
	}
	if !strings.Contains(s, "- simpledeploy-public") {
		t.Errorf("should add simpledeploy-public:\n%s", s)
	}
}

func TestInjectSharedNetworkMapForm(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: example.com
    networks:
      default: {}
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	s := string(out)
	if !strings.Contains(s, "default:") {
		t.Errorf("should preserve default:\n%s", s)
	}
	if !strings.Contains(s, "simpledeploy-public:") {
		t.Errorf("should add simpledeploy-public:\n%s", s)
	}
}

func TestInjectSharedNetworkMultiService(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      simpledeploy.endpoints.0.domain: example.com
  db:
    image: postgres
  api:
    image: node
    labels:
      simpledeploy.endpoints.0.domain: api.example.com
      simpledeploy.endpoints.0.port: "8080"
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	s := string(out)
	// Count attachments of simpledeploy-public under services (sequence items)
	n := strings.Count(s, "- simpledeploy-public")
	if n != 2 {
		t.Errorf("want 2 service attachments, got %d:\n%s", n, s)
	}
}

func TestInjectSharedNetworkListLabelsForm(t *testing.T) {
	src := []byte(`services:
  web:
    image: nginx
    labels:
      - simpledeploy.endpoints.0.domain=example.com
`)
	out, changed, err := InjectSharedNetwork(src, networkName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if !strings.Contains(string(out), "- simpledeploy-public") {
		t.Errorf("list-form labels should also trigger attachment:\n%s", out)
	}
}
