package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/audit"
)

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if err := s.reconciler.CancelOne(r.Context(), slug); err != nil {
		httpError(w, err, http.StatusNotFound)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"name": slug})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "deploy",
		Action:   "cancelled",
		AppSlug:  slug,
		After:    afterJSON,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}
