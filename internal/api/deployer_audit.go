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
	after, _ := json.Marshal(map[string]any{
		"version": e.Version,
		"error":   e.Error,
	})
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
