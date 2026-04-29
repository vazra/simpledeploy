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

	// Lockout check. Keyed on (user, ip) tuple plus a per-IP counter so
	// an attacker on one IP cannot lock out a victim's username globally
	// (account-DoS vector reported by audit L-05). The per-IP counter still
	// stops brute-force from a single source.
	lockoutKey := "user:" + req.Username + "@" + ip
	if s.lockout != nil {
		if s.lockout.IsLocked(lockoutKey) || s.lockout.IsLocked("ip:"+ip) {
			// Match the 401 response shape of an invalid-credentials reply
			// so a probe cannot use response variance to enumerate accounts.
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
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
			s.lockout.RecordFailure(lockoutKey)
			s.lockout.RecordFailure("ip:" + ip)
		}
		failCtx := audit.With(r.Context(), audit.Ctx{
			ActorName:   req.Username,
			ActorSource: "ui",
			IP:          ip,
		})
		_, _ = s.audit.Record(failCtx, audit.RecordReq{
			Category: "auth",
			Action:   "login_failed",
		})
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if s.jwt == nil {
		http.Error(w, "jwt not configured", http.StatusInternalServerError)
		return
	}

	token, err := s.jwt.Generate(user.ID, user.Username, user.Role, user.TokenVersion)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	if s.lockout != nil {
		s.lockout.RecordSuccess(lockoutKey)
		s.lockout.RecordSuccess("ip:" + ip)
	}

	okCtx := audit.With(r.Context(), audit.Ctx{
		ActorUserID: &user.ID,
		ActorName:   req.Username,
		ActorSource: "ui",
		IP:          ip,
	})
	_, _ = s.audit.Record(okCtx, audit.RecordReq{
		Category: "auth",
		Action:   "login_succeeded",
	})

	secure := s.tlsMode != "off"
	// Always SameSite=Strict regardless of TLS mode. Lax allows top-level
	// GET navigations to carry the cookie, which on a misconfigured TLS-off
	// install opens minor CSRF surface; Strict closes it. This breaks
	// link-bookmark sign-ins (visiting a bookmarked URL won't carry the
	// cookie on the very first navigation), which is acceptable for an
	// admin tool.
	sameSite := http.SameSiteStrictMode
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   86400, // 24h, matches JWT expiry
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Server-side invalidation: bump the user's token_version so the JWT
	// embedded in this (or any) outstanding cookie is rejected on its
	// next presentation. The route is unauthenticated (so logout works
	// after the cookie expires too); inspect the cookie best-effort.
	if cookie, err := r.Cookie("session"); err == nil && s.jwt != nil {
		if claims, err := s.jwt.Validate(cookie.Value); err == nil {
			_ = s.store.BumpTokenVersion(claims.UserID)
		}
	}
	secure := s.tlsMode != "off"
	// Always SameSite=Strict regardless of TLS mode. Lax allows top-level
	// GET navigations to carry the cookie, which on a misconfigured TLS-off
	// install opens minor CSRF surface; Strict closes it. This breaks
	// link-bookmark sign-ins (visiting a bookmarked URL won't carry the
	// cookie on the very first navigation), which is acceptable for an
	// admin tool.
	sameSite := http.SameSiteStrictMode
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
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

	user, err := s.store.CreateUser(req.Username, hash, "super_admin", "", "")
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
