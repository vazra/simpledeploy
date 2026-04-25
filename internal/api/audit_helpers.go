package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/store"
)

// recordAudit is a convenience wrapper around s.audit.Record that handles
// JSON marshalling of before/after snapshots and nil-app handling.
// app may be nil for system-level events.
func (s *Server) recordAudit(r *http.Request, app *store.App, category, action string, before, after any) {
	var b, a []byte
	if before != nil {
		b, _ = json.Marshal(before)
	}
	if after != nil {
		a, _ = json.Marshal(after)
	}
	req := audit.RecordReq{
		Category: category,
		Action:   action,
		Before:   b,
		After:    a,
	}
	if app != nil {
		req.AppID = &app.ID
		req.AppSlug = app.Slug
	}
	_, _ = s.audit.Record(r.Context(), req)
}
