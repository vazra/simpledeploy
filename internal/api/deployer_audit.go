package api

import (
	"context"
	"encoding/json"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/deployer"
)

// DeployerAuditAdapter converts deployer.DeployAuditEvent to audit.RecordReq
// and forwards it to the audit.Recorder. It is the concrete impl of
// deployer.AuditEmitter injected at startup so internal/deployer stays
// decoupled from internal/audit.
type DeployerAuditAdapter struct {
	Rec *audit.Recorder
}

func (a *DeployerAuditAdapter) RecordDeploy(ctx context.Context, e deployer.DeployAuditEvent) {
	if a == nil || a.Rec == nil {
		return
	}
	view := map[string]any{"error": e.Error}
	// Version is 0 for normal deploys (the compose version is stored after
	// the deployer returns); leave it out rather than record "version 0".
	if e.Version > 0 {
		view["version"] = e.Version
	}
	after, _ := json.Marshal(view)
	var appID *int64
	if e.AppID != 0 {
		id := e.AppID
		appID = &id
	}
	_, _ = a.Rec.Record(ctx, audit.RecordReq{
		AppID:            appID,
		AppSlug:          e.AppSlug,
		Category:         "deploy",
		Action:           e.Action,
		After:            after,
		Error:            e.Error,
		ComposeVersionID: e.ComposeVersionID,
	})
}
