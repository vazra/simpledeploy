package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/store"
)

type Server struct {
	mux         *http.ServeMux
	port        int
	store       *store.Store
	jwt         *auth.JWTManager
	rateLimiter *auth.RateLimiter
}

func NewServer(port int, st *store.Store, jwtMgr *auth.JWTManager, rl *auth.RateLimiter) *Server {
	s := &Server{
		mux:         http.NewServeMux(),
		port:        port,
		store:       st,
		jwt:         jwtMgr,
		rateLimiter: rl,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Public routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Protected routes
	s.mux.Handle("GET /api/apps", s.authMiddleware(
		http.HandlerFunc(s.handleListApps)))
	s.mux.Handle("GET /api/apps/{slug}", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleGetApp))))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.mux)
}
