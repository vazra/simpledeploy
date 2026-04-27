package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/vazra/simpledeploy/internal/recipes"
)

func (s *Server) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	if s.recipesCache == nil {
		http.Error(w, "recipes not configured", http.StatusServiceUnavailable)
		return
	}
	idx, err := s.recipesCache.Index(r.Context())
	if err != nil {
		http.Error(w, "recipes catalog unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=60")
	_ = json.NewEncoder(w).Encode(idx)
}

func (s *Server) handleFetchRecipeFile(w http.ResponseWriter, r *http.Request) {
	if s.recipesCache == nil {
		http.Error(w, "recipes not configured", http.StatusServiceUnavailable)
		return
	}
	id := r.URL.Query().Get("id")
	file := r.URL.Query().Get("file")
	if file == "" {
		file = "compose"
	}
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	idx, err := s.recipesCache.Index(r.Context())
	if err != nil {
		http.Error(w, "recipes catalog unavailable", http.StatusBadGateway)
		return
	}
	var rec *recipes.Recipe
	for i := range idx.Recipes {
		if idx.Recipes[i].ID == id {
			rec = &idx.Recipes[i]
			break
		}
	}
	if rec == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var url string
	switch file {
	case "compose":
		url = rec.ComposeURL
	case "readme":
		url = rec.ReadmeURL
	default:
		http.Error(w, "invalid file", http.StatusBadRequest)
		return
	}
	body, err := s.recipesCache.Client().FetchText(r.Context(), url)
	if err != nil {
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}
	// Telemetry: a compose pull is the actual "use this recipe" intent.
	if file == "compose" && s.store != nil {
		if err := s.store.RecordRecipePull(context.Background(), id); err != nil {
			log.Printf("recipe pull telemetry failed: %v", err)
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleRecipePopularity(w http.ResponseWriter, r *http.Request) {
	if user := requireRole(w, r, "super_admin"); user == nil {
		return
	}
	if s.store == nil {
		http.Error(w, "store unavailable", http.StatusInternalServerError)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	counts, err := s.store.RecipePullCounts(r.Context(), limit)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"recipes": counts})
}
