package mirror

import (
	"strings"
	"testing"
)

func TestRewriteRef(t *testing.T) {
	const prefix = "ghcr.io/vazra/"
	cases := []struct {
		in       string
		want     string
		wantSkip bool
	}{
		// Docker Hub official (no namespace)
		{"nginx:alpine", "ghcr.io/vazra/nginx:alpine", false},
		{"nginx", "ghcr.io/vazra/nginx", false},
		{"postgres:16-alpine", "ghcr.io/vazra/postgres:16-alpine", false},
		// Docker Hub user/org
		{"louislam/uptime-kuma:1.23", "ghcr.io/vazra/louislam/uptime-kuma:1.23", false},
		{"woodpeckerci/woodpecker-agent:v2.7", "ghcr.io/vazra/woodpeckerci/woodpecker-agent:v2.7", false},
		// Explicit docker.io
		{"docker.io/library/nginx:alpine", "ghcr.io/vazra/nginx:alpine", false},
		{"docker.io/louislam/uptime-kuma:1", "ghcr.io/vazra/louislam/uptime-kuma:1", false},
		// Digest refs preserved
		{"nginx@sha256:abc123", "ghcr.io/vazra/nginx@sha256:abc123", false},
		// Non-docker.io registries - leave alone
		{"ghcr.io/gethomepage/homepage:latest", "", true},
		{"quay.io/foo/bar:latest", "", true},
		{"docker.dragonflydb.io/dragonflydb/dragonfly:latest", "", true},
		{"myregistry.local:5000/foo/bar", "", true},
		{"localhost/test:latest", "", true},
		// Already mirrored - idempotent
		{"ghcr.io/vazra/nginx:alpine", "", true},
		// Empty
		{"", "", true},
	}
	for _, tc := range cases {
		got, ok := rewriteRef(tc.in, prefix)
		if tc.wantSkip {
			if ok {
				t.Errorf("rewriteRef(%q): expected skip, got %q", tc.in, got)
			}
			continue
		}
		if !ok {
			t.Errorf("rewriteRef(%q): expected rewrite, got skip", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("rewriteRef(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRewriteCompose(t *testing.T) {
	const prefix = "ghcr.io/vazra/"
	in := `services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
  api:
    image: "node:20-alpine"
    environment:
      NODE_ENV: production
  db:
    image: 'postgres:16-alpine'
  cache:
    image: redis:7-alpine  # inline comment
  external:
    image: ghcr.io/foo/bar:latest
  multi:
    image: woodpeckerci/woodpecker-agent:v2.7
`
	want := `services:
  web:
    image: ghcr.io/vazra/nginx:alpine
    ports:
      - "80:80"
  api:
    image: "ghcr.io/vazra/node:20-alpine"
    environment:
      NODE_ENV: production
  db:
    image: 'ghcr.io/vazra/postgres:16-alpine'
  cache:
    image: ghcr.io/vazra/redis:7-alpine  # inline comment
  external:
    image: ghcr.io/foo/bar:latest
  multi:
    image: ghcr.io/vazra/woodpeckerci/woodpecker-agent:v2.7
`
	got := string(RewriteCompose([]byte(in), prefix))
	if got != want {
		t.Errorf("RewriteCompose mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRewriteComposeNoPrefix(t *testing.T) {
	in := []byte("services:\n  web:\n    image: nginx:alpine\n")
	got := RewriteCompose(in, "")
	if string(got) != string(in) {
		t.Errorf("RewriteCompose with empty prefix must be a no-op")
	}
}

func TestRewriteComposeIdempotent(t *testing.T) {
	const prefix = "ghcr.io/vazra/"
	in := []byte("services:\n  web:\n    image: nginx:alpine\n")
	once := RewriteCompose(in, prefix)
	twice := RewriteCompose(once, prefix)
	if string(once) != string(twice) {
		t.Errorf("RewriteCompose not idempotent:\nonce:  %s\ntwice: %s", once, twice)
	}
}

func TestRewriteComposeAutoAddsSlash(t *testing.T) {
	in := []byte("services:\n  web:\n    image: nginx:alpine\n")
	withSlash := RewriteCompose(in, "ghcr.io/vazra/")
	withoutSlash := RewriteCompose(in, "ghcr.io/vazra")
	if string(withSlash) != string(withoutSlash) {
		t.Errorf("RewriteCompose should tolerate missing trailing slash")
	}
	if !strings.Contains(string(withSlash), "ghcr.io/vazra/nginx:alpine") {
		t.Errorf("expected rewritten image in output, got: %s", withSlash)
	}
}

func TestRewritePortsLoopback(t *testing.T) {
	in := `services:
  web:
    image: nginx
    ports:
      - "8080:80"
      - 9090:90
      - "5432:5432/tcp"
`
	out := string(RewritePortsLoopback([]byte(in)))
	wantSubs := []string{
		`- "127.0.0.1:8080:80"`,
		`- 127.0.0.1:9090:90`,
		`- "127.0.0.1:5432:5432/tcp"`,
	}
	for _, w := range wantSubs {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q\n--- got ---\n%s", w, out)
		}
	}
}

func TestRewritePortsLoopback_LeavesExplicitInterface(t *testing.T) {
	in := `services:
  web:
    ports:
      - "0.0.0.0:8080:80"
      - "127.0.0.1:9090:90"
      - "[::1]:5432:5432"
`
	out := string(RewritePortsLoopback([]byte(in)))
	if out != in {
		t.Errorf("operator-explicit interface bindings must not be rewritten:\n--- got ---\n%s\n--- want ---\n%s", out, in)
	}
}

func TestRewritePortsLoopback_LeavesContainerOnly(t *testing.T) {
	in := `services:
  web:
    ports:
      - 80
`
	out := string(RewritePortsLoopback([]byte(in)))
	if out != in {
		t.Errorf("container-only ports must not be rewritten")
	}
}
