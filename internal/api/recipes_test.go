package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/recipes"
)

func newRecipesUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/index.json") || r.URL.Path == "/" {
			_ = json.NewEncoder(w).Encode(recipes.Index{
				SchemaVersion: 1,
				GeneratedAt:   "x",
				Recipes: []recipes.Recipe{{
					ID: "a", Name: "A", Category: "web", Description: "d",
					ComposeURL: "http://" + r.Host + "/recipes/a/compose.yml",
					ReadmeURL:  "http://" + r.Host + "/recipes/a/README.md",
				}},
			})
			return
		}
		w.Write([]byte("services: {}\n"))
	}))
}

func TestHandleListRecipes(t *testing.T) {
	upstream := newRecipesUpstream(t)
	defer upstream.Close()
	cli := recipes.NewClient(upstream.URL+"/index.json", 0)
	s := &Server{recipesCache: recipes.NewCache(cli, time.Minute)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/recipes/community", nil)
	s.handleListRecipes(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), `"id":"a"`) {
		t.Fatalf("body: %s", body)
	}
}

func TestHandleListRecipesUpstreamError(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()
	cli := recipes.NewClient(bad.URL+"/index.json", 0)
	s := &Server{recipesCache: recipes.NewCache(cli, time.Minute)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/recipes/community", nil)
	s.handleListRecipes(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", rec.Code)
	}
}

func TestHandleFetchRecipeFileMissingID(t *testing.T) {
	upstream := newRecipesUpstream(t)
	defer upstream.Close()
	cli := recipes.NewClient(upstream.URL+"/index.json", 0)
	s := &Server{recipesCache: recipes.NewCache(cli, time.Minute)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/recipes/community/file", nil)
	s.handleFetchRecipeFile(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestHandleFetchRecipeFileNotFound(t *testing.T) {
	upstream := newRecipesUpstream(t)
	defer upstream.Close()
	cli := recipes.NewClient(upstream.URL+"/index.json", 0)
	s := &Server{recipesCache: recipes.NewCache(cli, time.Minute)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/recipes/community/file?id=does-not-exist", nil)
	s.handleFetchRecipeFile(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
