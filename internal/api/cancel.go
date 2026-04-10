package api

import (
	"encoding/json"
	"net/http"
)

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := s.reconciler.CancelOne(r.Context(), slug); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}
