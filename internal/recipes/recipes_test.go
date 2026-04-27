package recipes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Index{
			SchemaVersion: 1,
			GeneratedAt:   "2026-01-01T00:00:00Z",
			Recipes: []Recipe{{ID: "nginx-static", Name: "Nginx", Category: "web",
				Description: "x", ComposeURL: "u", ReadmeURL: "r"}},
		})
	}))
	defer srv.Close()
	c := NewClient(srv.URL, 0)
	idx, err := c.FetchIndex(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Recipes) != 1 || idx.Recipes[0].ID != "nginx-static" {
		t.Fatalf("unexpected: %+v", idx)
	}
}

func TestFetchIndexRejectsWrongSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"schema_version":2,"generated_at":"x","recipes":[]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, 0)
	if _, err := c.FetchIndex(t.Context()); err == nil {
		t.Fatal("expected schema error")
	}
}

func TestFetchTextWithinBase(t *testing.T) {
	body := "services:\n  web:\n    image: nginx\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/index.json", 0)
	got, err := c.FetchText(t.Context(), srv.URL+"/some/file.yml")
	if err != nil {
		t.Fatal(err)
	}
	if got != body {
		t.Fatalf("body mismatch")
	}
}

func TestFetchTextRejectsForeignHost(t *testing.T) {
	c := NewClient("https://example.com/index.json", 0)
	if _, err := c.FetchText(t.Context(), "https://evil.com/x"); err == nil {
		t.Fatal("expected host rejection")
	}
}

func TestFetchTextRejectsLargeBody(t *testing.T) {
	big := strings.Repeat("a", maxRecipeFileBytes+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(big))
	}))
	defer srv.Close()
	c := NewClient(srv.URL+"/index.json", 0)
	if _, err := c.FetchText(t.Context(), srv.URL+"/big.yml"); err == nil {
		t.Fatal("expected size error")
	}
}
