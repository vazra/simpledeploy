package client

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListApps(t *testing.T) {
	apps := []AppInfo{
		{ID: 1, Name: "myapp", Slug: "myapp", Status: "running", Domain: "myapp.example.com"},
		{ID: 2, Name: "other", Slug: "other", Status: "stopped", Domain: ""},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/apps" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(apps)
	}))
	defer srv.Close()

	c := New(srv.URL, "sd_test")
	got, err := c.ListApps()
	if err != nil {
		t.Fatalf("ListApps error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(got))
	}
	if got[0].Name != "myapp" {
		t.Errorf("expected name myapp, got %s", got[0].Name)
	}
	if got[1].Slug != "other" {
		t.Errorf("expected slug other, got %s", got[1].Slug)
	}
}

func TestClientDeployApp(t *testing.T) {
	var gotName, gotCompose string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/apps/deploy" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotName = body["name"]
		gotCompose = body["compose"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "sd_test")
	composeData := []byte("version: '3'\nservices:\n  web:\n    image: nginx\n")
	if err := c.DeployApp("myapp", composeData); err != nil {
		t.Fatalf("DeployApp error: %v", err)
	}
	if gotName != "myapp" {
		t.Errorf("expected name myapp, got %s", gotName)
	}
	decoded, err := base64.StdEncoding.DecodeString(gotCompose)
	if err != nil {
		t.Fatalf("compose not valid base64: %v", err)
	}
	if string(decoded) != string(composeData) {
		t.Errorf("compose mismatch: got %s", string(decoded))
	}
}

func TestClientAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]AppInfo{})
	}))
	defer srv.Close()

	c := New(srv.URL, "sd_mykey123")
	c.ListApps()

	if gotAuth != "Bearer sd_mykey123" {
		t.Errorf("expected 'Bearer sd_mykey123', got %q", gotAuth)
	}
}

func TestClientAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "app not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "sd_test")
	_, err := c.GetApp("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// check contains status code
	expected := "API error 404"
	if len(err.Error()) < len(expected) || err.Error()[:len(expected)] != expected {
		t.Errorf("expected error starting with %q, got %q", expected, err.Error())
	}
}

func TestClientRemoveApp(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "sd_test")
	if err := c.RemoveApp("myapp"); err != nil {
		t.Fatalf("RemoveApp error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/api/apps/myapp" {
		t.Errorf("expected /api/apps/myapp, got %s", gotPath)
	}
}
