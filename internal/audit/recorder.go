package audit

import (
	"context"

	"github.com/vazra/simpledeploy/internal/audit/render"
	"github.com/vazra/simpledeploy/internal/store"
)

// Recorder writes audit events to the DB-backed audit_log table.
type Recorder struct {
	s *store.Store
}

// NewRecorder creates a Recorder backed by the given store.
func NewRecorder(s *store.Store) *Recorder { return &Recorder{s: s} }

// RecordReq is the input to Recorder.Record.
type RecordReq struct {
	AppID            *int64
	AppSlug          string
	Category         string
	Action           string
	Before           []byte
	After            []byte
	Error            string
	ComposeVersionID *int64
}

var syncEligibleCategories = map[string]bool{
	"compose":  true,
	"endpoint": true,
	"backup":   true,
	"alert":    true,
	"webhook":  true,
	"registry": true,
	"access":   true,
	"env":      true,
}

var syncEligibleLifecycleActions = map[string]bool{
	"created": true,
	"renamed": true,
	"removed": true,
}

// Record persists a single audit event. It is a no-op when r is nil.
func (r *Recorder) Record(ctx context.Context, req RecordReq) (int64, error) {
	if r == nil {
		return 0, nil
	}
	c := From(ctx)
	summary, target := render.Render(req.Category, req.Action, req.Before, req.After)
	eligible := syncEligibleCategories[req.Category]
	if req.Category == "lifecycle" && syncEligibleLifecycleActions[req.Action] {
		eligible = true
	}
	return r.s.RecordAudit(ctx, store.AuditEntry{
		AppID:            req.AppID,
		AppSlug:          req.AppSlug,
		ActorUserID:      c.ActorUserID,
		ActorName:        c.ActorName,
		ActorSource:      c.ActorSource,
		IP:               c.IP,
		Category:         req.Category,
		Action:           req.Action,
		Target:           target,
		Summary:          summary,
		BeforeJSON:       req.Before,
		AfterJSON:        req.After,
		Error:            req.Error,
		ComposeVersionID: req.ComposeVersionID,
		SyncEligible:     eligible,
	})
}
