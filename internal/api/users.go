package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
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
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	Role        string    `json:"role"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	CreatedAt   time.Time `json:"created_at"`
}

func toUserResponse(u *store.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role, DisplayName: u.DisplayName, Email: u.Email, CreatedAt: u.CreatedAt}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin", "admin") == nil {
		return
	}
	users, err := s.store.ListUsers()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = toUserResponse(&u)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(authUser.ID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toUserResponse(user))
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateProfile(authUser.ID, body.DisplayName, body.Email); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.NewPassword == "" {
		http.Error(w, "new password required", http.StatusBadRequest)
		return
	}
	user, err := s.store.GetUserByID(authUser.ID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
		http.Error(w, "current password is incorrect", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdatePassword(authUser.ID, hash); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "password_changed", Username: authUser.Username, Success: true})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Role        string `json:"role"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if len(body.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}
	if body.Email != "" {
		if taken, _ := s.store.EmailTaken(body.Email, 0); taken {
			http.Error(w, "email already in use", http.StatusConflict)
			return
		}
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	u, err := s.store.CreateUser(body.Username, hash, body.Role, body.DisplayName, body.Email)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			http.Error(w, "username already exists", http.StatusConflict)
			return
		}
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		caller := GetAuthUser(r)
		s.audit.Log(audit.Event{Type: "user_created", Username: caller.Username, Detail: body.Username + " (" + body.Role + ")", Success: true})
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toUserResponse(u))
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	caller := requireRole(w, r, "super_admin")
	if caller == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		DisplayName     string `json:"display_name"`
		Email           string `json:"email"`
		Role            string `json:"role"`
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.Email != "" {
		if taken, _ := s.store.EmailTaken(body.Email, id); taken {
			http.Error(w, "email already in use", http.StatusConflict)
			return
		}
	}
	if err := s.store.UpdateProfile(id, body.DisplayName, body.Email); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if body.Role != "" {
		if caller.ID == id && body.Role != "super_admin" {
			http.Error(w, "cannot change your own role", http.StatusBadRequest)
			return
		}
		if err := s.store.UpdateUserRole(id, body.Role); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
	if body.Password != "" {
		if len(body.Password) < 8 {
			http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
			return
		}
		// Always require caller's password to reset any user's password
		if body.CurrentPassword == "" {
			http.Error(w, "your password is required to reset passwords", http.StatusBadRequest)
			return
		}
		callerUser, err := s.store.GetUserByID(caller.ID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if !auth.CheckPassword(callerUser.PasswordHash, body.CurrentPassword) {
			http.Error(w, "your password is incorrect", http.StatusBadRequest)
			return
		}
		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		if err := s.store.UpdatePassword(id, hash); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
	user, err := s.store.GetUserByID(id)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "user_updated", Username: caller.Username, Detail: user.Username, Success: true})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toUserResponse(user))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	caller := requireRole(w, r, "super_admin")
	if caller == nil {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if caller.ID == id {
		http.Error(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteUser(id); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		caller := GetAuthUser(r)
		s.audit.Log(audit.Event{Type: "user_deleted", Username: caller.Username, Detail: fmt.Sprintf("user_id=%d", id), Success: true})
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
	plaintext, hash, err := auth.GenerateAPIKey(s.masterSecret)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	k, err := s.store.CreateAPIKey(user.ID, hash, body.Name)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "apikey_created", Username: user.Username, Detail: body.Name, Success: true})
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
	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "apikey_deleted", Username: user.Username, Detail: fmt.Sprintf("key_id=%d", id), Success: true})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
