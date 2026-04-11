package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
)

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := auth.RealIP(r, s.trustedProxies)

	// Rate limit by client IP
	if s.rateLimiter != nil {
		if !s.rateLimiter.Allow(ip) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Lockout check
	if s.lockout != nil {
		if s.lockout.IsLocked("user:"+req.Username) || s.lockout.IsLocked("ip:"+ip) {
			http.Error(w, "account temporarily locked", http.StatusTooManyRequests)
			return
		}
	}

	// Always run bcrypt to prevent user enumeration via timing
	user, err := s.store.GetUserByUsername(req.Username)
	// Use a dummy hash so bcrypt runs even if user not found
	hash := "$2a$12$000000000000000000000uGWDRFaOZaHVkxgcvqcEnF8VjqDBqyq"
	if err == nil {
		hash = user.PasswordHash
	}
	if !auth.CheckPassword(hash, req.Password) || err != nil {
		if s.lockout != nil {
			s.lockout.RecordFailure("user:" + req.Username)
			s.lockout.RecordFailure("ip:" + ip)
		}
		if s.audit != nil {
			s.audit.Log(audit.Event{Type: "login_failed", Username: req.Username, IP: ip})
		}
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if s.jwt == nil {
		http.Error(w, "jwt not configured", http.StatusInternalServerError)
		return
	}

	token, err := s.jwt.Generate(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	if s.lockout != nil {
		s.lockout.RecordSuccess("user:" + req.Username)
		s.lockout.RecordSuccess("ip:" + ip)
	}

	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "login", Username: user.Username, IP: ip, Success: true})
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24h, matches JWT expiry
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.UserCount()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"needs_setup": count == 0})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.UserCount()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "conflict", http.StatusConflict)
		return
	}

	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := s.store.CreateUser(req.Username, hash, "super_admin")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if req.DisplayName != "" || req.Email != "" {
		if err := s.store.UpdateProfile(user.ID, req.DisplayName, req.Email); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}
