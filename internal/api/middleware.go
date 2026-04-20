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
				// Verify user still exists in DB
				user, err := s.store.GetUserByID(claims.UserID)
				if err != nil || user == nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				r = setAuthUser(r, &AuthUser{ID: user.ID, Username: user.Username, Role: user.Role})
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

// checkAppAccessByID verifies the authenticated user has access to appID.
// Writes 401/403 to w on failure and returns false.
func (s *Server) checkAppAccessByID(w http.ResponseWriter, r *http.Request, appID int64) bool {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if user.Role == "super_admin" {
		return true
	}
	ok, _ := s.store.HasAppAccessByID(user.ID, appID)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return false
	}
	return true
}

// rateLimitMiddleware applies the server-level rate limiter to a handler.
// Used for unauthenticated endpoints that should still be rate-limited (e.g. git webhook).
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter != nil {
			ip := auth.RealIP(r, s.trustedProxies)
			if !s.rateLimiter.Allow(ip) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// superAdminMiddleware requires the caller to have role "super_admin".
// Use for destructive system-wide operations (vacuum, prune, audit clear).
func (s *Server) superAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetAuthUser(r)
		if user == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user.Role != "super_admin" {
			http.Error(w, "forbidden: super_admin required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
