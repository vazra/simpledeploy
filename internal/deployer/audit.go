package deployer

import "context"

// AuditEmitter records deploy outcomes. Concrete impl is a thin adapter in
// cmd/simpledeploy/main.go that converts to audit.RecordReq. Defined here as
// an interface so internal/deployer does not import internal/audit.
type AuditEmitter interface {
	RecordDeploy(ctx context.Context, e DeployAuditEvent)
}

// DeployAuditEvent is the payload passed to AuditEmitter.RecordDeploy.
type DeployAuditEvent struct {
	AppID            int64  // 0 when unknown (deployer layer has no DB access)
	AppSlug          string
	Action           string // "deploy_succeeded", "deploy_failed", "rollback"
	Version          int
	ComposeVersionID *int64
	Error            string
}
