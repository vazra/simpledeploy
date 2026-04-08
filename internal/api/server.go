package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vazra/simpledeploy/internal/store"
)

type Server struct {
	mux   *http.ServeMux
	port  int
	store *store.Store
}

func NewServer(port int, st *store.Store) *Server {
	s := &Server{
		mux:   http.NewServeMux(),
		port:  port,
		store: st,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
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
