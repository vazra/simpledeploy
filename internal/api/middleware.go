package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"

type AuthUser struct {
	ID       int64
	Username string
	Role     string
}

func GetAuthUser(r *http.Request) *AuthUser {
	u, _ := r.Context().Value(userContextKey).(*AuthUser)
	return u
}

func setAuthUser(r *http.Request, user *AuthUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey, user))
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try API key first (Authorization: Bearer sd_...)
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			keyHash := auth.HashAPIKey(token, s.masterSecret)
			keyRecord, user, err := s.store.GetAPIKeyByHash(keyHash)
			if err == nil && keyRecord != nil {
				// Check API key expiry
				if keyRecord.ExpiresAt != nil && time.Now().After(*keyRecord.ExpiresAt) {
					http.Error(w, "api key expired", http.StatusUnauthorized)
					return
				}
				r = setAuthUser(r, &AuthUser{ID: user.ID, Username: user.Username, Role: user.Role})
				next.ServeHTTP(w, r)
				return
			}
		}

		// Try JWT cookie
		cookie, err := r.Cookie("session")
		if err == nil && s.jwt != nil {
			claims, err := s.jwt.Validate(cookie.Value)
			if err == nil {
				r = setAuthUser(r, &AuthUser{ID: claims.UserID, Username: claims.Username, Role: claims.Role})
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

func (s *Server) appAccessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetAuthUser(r)
		if user == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// super_admin bypasses
		if user.Role == "super_admin" {
			next.ServeHTTP(w, r)
			return
		}

		slug := r.PathValue("slug")
		if slug == "" {
			next.ServeHTTP(w, r)
			return
		}

		hasAccess, _ := s.store.HasAppAccess(user.ID, slug)
		if !hasAccess {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}
