package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/store"
)

// parseLimit parses "limit" query param, clamped to [1, max], defaulting to def.
func parseLimit(r *http.Request, def, max int) int {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// parseBefore parses the "before" cursor (entry ID) from query params.
func parseBefore(r *http.Request) int64 {
	s := r.URL.Query().Get("before")
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseCategories splits the "categories" query param on commas.
func parseCategories(r *http.Request) []string {
	s := r.URL.Query().Get("categories")
	if s == "" {
		return nil
	}
	var out []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

// buildNonAdminFilter populates AllowedAppIDs and ActorUserID for a non-super_admin caller.
// Non-admins see entries for their accessible apps plus their own auth events only.
// System category entries are admin-only.
func (s *Server) buildNonAdminFilter(r *http.Request, f *store.ActivityFilter) error {
	user := GetAuthUser(r)
	ids, err := s.store.AccessibleAppIDs(r.Context(), user.ID)
	if err != nil {
		return err
	}
	if ids == nil {
		ids = []int64{} // empty slice = only auth self-rows
	}
	f.AllowedAppIDs = ids
	f.ActorUserID = &user.ID
	return nil
}

func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	f := store.ActivityFilter{
		Limit:      parseLimit(r, 50, 200),
		Before:     parseBefore(r),
		Categories: parseCategories(r),
	}
	if slug := r.URL.Query().Get("app"); slug != "" {
		f.AppSlug = slug
		// Non-admins: verify they can access the requested app slug.
		if user.Role != "super_admin" {
			app, err := s.store.GetAppBySlug(slug)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			ok, _ := s.store.HasAppAccessByID(user.ID, app.ID)
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
	}
	if user.Role != "super_admin" {
		if err := s.buildNonAdminFilter(r, &f); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
	entries, next, err := s.store.ListActivity(r.Context(), f)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"entries": entries, "next_before": next})
}

func (s *Server) handleAppActivity(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !s.checkAppAccessByID(w, r, app.ID) {
		return
	}
	// Filter by slug so rows recorded with only AppSlug (when AppID was not
	// available at handler time) are included. Slug is unique among live apps.
	_ = app
	f := store.ActivityFilter{
		AppSlug:    slug,
		Categories: parseCategories(r),
		Limit:      parseLimit(r, 50, 200),
		Before:     parseBefore(r),
	}
	entries, next, err := s.store.ListActivity(r.Context(), f)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"entries": entries, "next_before": next})
}

func (s *Server) handleRecentActivity(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	f := store.ActivityFilter{Limit: parseLimit(r, 8, 50)}
	if user.Role != "super_admin" {
		if err := s.buildNonAdminFilter(r, &f); err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
	}
	entries, _, err := s.store.ListActivity(r.Context(), f)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []store.AuditEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}

func (s *Server) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	e, err := s.store.GetActivity(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Mirror the non-admin filter applied to ListActivity:
	//   - super_admin: any row
	//   - other roles: app-scoped row only if the caller has access to the app;
	//     system-scoped row (AppID == nil) only if the caller is the actor.
	user := GetAuthUser(r)
	if user.Role != "super_admin" {
		if e.AppID != nil {
			if !s.checkAppAccessByID(w, r, *e.AppID) {
				return
			}
		} else {
			if e.ActorUserID == nil || *e.ActorUserID != user.ID {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

func (s *Server) handleGetAuditConfig(w http.ResponseWriter, r *http.Request) {
	days, err := s.store.GetAuditRetentionDays(r.Context())
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"retention_days": days})
}

func (s *Server) handlePutAuditConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if body.RetentionDays < 0 {
		http.Error(w, "retention_days must be >= 0", http.StatusBadRequest)
		return
	}
	if err := s.store.SetAuditRetentionDays(r.Context(), body.RetentionDays); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePurgeActivity(w http.ResponseWriter, r *http.Request) {
	// Count rows about to be wiped so the sentinel records the scale of the
	// purge — useful when correlating with operator reports.
	count, _ := s.store.CountAudit(r.Context())
	if err := s.store.PurgeAudit(r.Context()); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	// Sentinel: write an audit row AFTER the purge so it survives. Without
	// this, a super_admin could clear the audit log with no record of who
	// did it. We bypass the optional Recorder and write directly via the
	// store so the row is persisted even in setups where SetAudit has not
	// been called.
	c := audit.From(r.Context())
	beforeJSON, _ := json.Marshal(map[string]any{"row_count": count})
	_, _ = s.store.RecordAudit(r.Context(), store.AuditEntry{
		ActorUserID: c.ActorUserID,
		ActorName:   c.ActorName,
		ActorSource: c.ActorSource,
		IP:          c.IP,
		Category:    "system",
		Action:      "audit_purged",
		Summary:     "audit log purged",
		BeforeJSON:  beforeJSON,
	})
	w.WriteHeader(http.StatusNoContent)
}
