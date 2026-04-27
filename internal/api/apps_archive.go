package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

// handleListArchived returns archived apps with their tombstone payload (if any).
func (s *Server) handleListArchived(w http.ResponseWriter, r *http.Request) {
	apps, err := s.store.ListArchivedApps()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	type archivedApp struct {
		store.App
		Tombstone *configsync.Tombstone `json:"tombstone"`
	}
	out := make([]archivedApp, 0, len(apps))
	for _, a := range apps {
		entry := archivedApp{App: a}
		if s.cs != nil {
			if t, err := s.cs.ReadTombstone(a.Slug); err == nil {
				entry.Tombstone = t
			} else if !os.IsNotExist(err) {
				// Log but tolerate; tombstone stays nil.
			}
		}
		out = append(out, entry)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// handlePurge permanently removes an archived app: DB row + history cascade and
// tombstone file. Returns 404 if app not found, 409 if app is not archived.
func (s *Server) handlePurge(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !validAppName.MatchString(slug) {
		http.Error(w, "invalid app name", http.StatusBadRequest)
		return
	}

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !app.ArchivedAt.Valid {
		http.Error(w, "app is not archived; use DELETE /api/apps/{slug}", http.StatusConflict)
		return
	}

	if err := s.store.PurgeApp(slug); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	if s.cs != nil {
		if err := s.cs.DeleteTombstone(slug); err != nil {
			// Non-fatal; row is already gone.
		}
	}

	after, _ := json.Marshal(map[string]any{"name": slug})
	_, _ = s.audit.Record(r.Context(), audit.RecordReq{
		Category: "lifecycle",
		Action:   "purged",
		AppSlug:  slug,
		After:    after,
	})

	w.WriteHeader(http.StatusNoContent)
}
