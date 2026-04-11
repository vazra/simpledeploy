package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

// requireRole checks auth and role, returns user or writes error and nil.
func requireRole(w http.ResponseWriter, r *http.Request, roles ...string) *AuthUser {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil
	}
	for _, role := range roles {
		if user.Role == role {
			return user
		}
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return nil
}

// userResponse omits password_hash from JSON output.
type userResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func toUserResponse(u *store.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	users, err := s.store.ListUsers()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = userResponse{ID: u.ID, Username: u.Username, Role: u.Role}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := s.store.CreateUser(body.Username, hash, body.Role)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toUserResponse(u))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteUser(id); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGrantAccess(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		AppSlug string `json:"app_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	app, err := s.store.GetAppBySlug(body.AppSlug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if err := s.store.GrantAppAccess(userID, app.ID); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleRevokeAccess(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if err := s.store.RevokeAppAccess(userID, app.ID); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	keys, err := s.store.ListAPIKeysByUser(user.ID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	type keyResponse struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	resp := make([]keyResponse, len(keys))
	for i, k := range keys {
		resp[i] = keyResponse{ID: k.ID, Name: k.Name}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	plaintext, hash, err := auth.GenerateAPIKey()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	k, err := s.store.CreateAPIKey(user.ID, hash, body.Name)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id":   k.ID,
		"name": k.Name,
		"key":  plaintext,
	})
}

func (s *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	// super_admin can delete any key; others only their own
	ownerID := user.ID
	if user.Role == "super_admin" {
		ownerID = 0
	}
	if err := s.store.DeleteAPIKey(id, ownerID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
