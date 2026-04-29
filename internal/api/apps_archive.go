package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

// handleListArchived returns archived apps with their tombstone payload (if any).
// super_admin sees all archived apps; manage/viewer only see archived apps they were
// granted access to.
func (s *Server) handleListArchived(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	apps, err := s.store.ListArchivedApps()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	var allowed map[string]struct{}
	if user.Role != "super_admin" {
		slugs, err := s.store.GetUserAppSlugs(user.ID)
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		allowed = make(map[string]struct{}, len(slugs))
		for _, sl := range slugs {
			allowed[sl] = struct{}{}
		}
	}

	type archivedApp struct {
		store.App
		Tombstone *configsync.Tombstone `json:"tombstone"`
	}
	out := make([]archivedApp, 0, len(apps))
	for _, a := range apps {
		if allowed != nil {
			if _, ok := allowed[a.Slug]; !ok {
				continue
			}
		}
		entry := archivedApp{App: a}
		if s.cs != nil {
			if t, err := s.cs.ReadTombstone(a.Slug); err == nil {
				entry.Tombstone = t
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
		// Best-effort; the row is already gone.
		_ = s.cs.DeleteTombstone(slug)
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
