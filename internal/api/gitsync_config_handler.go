package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/gitsync"
)

// gsMu guards gs and the reload operation. RLock for reads; Lock for reload.
var gsMu sync.RWMutex

// reconcilerRef is a callback set by main so ReloadGitSync can wire the
// reconciler into the new syncer. Set via SetReconcilerRef.
var reconcilerRef gitsync.Reconciler

// SetReconcilerRef sets the reconciler callback used when reloading gitsync.
func (s *Server) SetReconcilerRef(r gitsync.Reconciler) {
	reconcilerRef = r
}

// ReloadGitSync stops the current syncer (if any), rebuilds config from DB+YAML,
// and starts a new syncer. Returns the new status or an error.
func (s *Server) ReloadGitSync(ctx context.Context) (*gitsync.Status, error) {
	gsMu.Lock()
	defer gsMu.Unlock()

	// Stop existing syncer.
	if s.gs != nil {
		if err := s.gs.Stop(); err != nil {
			log.Printf("[gitsync] reload: stop existing: %v", err)
		}
		s.gs = nil
	}

	if s.cfg == nil || s.store == nil {
		disabled := gitsync.Status{Enabled: false}
		return &disabled, nil
	}

	var yamlGS *config.GitSyncConfig
	if s.cfg != nil {
		v := s.cfg.GitSync
		yamlGS = &v
	}

	gsCfg, err := gitsync.ResolveConfig(s.store, yamlGS, s.cfg.AppsDir, s.masterSecret)
	if err != nil {
		return nil, fmt.Errorf("resolve gitsync config: %w", err)
	}

	if !gsCfg.Enabled {
		st := gitsync.Status{Enabled: false}
		return &st, nil
	}

	gs, err := gitsync.New(*gsCfg, s.store, s.cs, reconcilerRef)
	if err != nil {
		return nil, fmt.Errorf("gitsync.New: %w", err)
	}
	if err := gs.Start(ctx); err != nil {
		return nil, fmt.Errorf("gitsync.Start: %w", err)
	}

	s.gs = gs

	// Re-wire configsync sidecar hook.
	if s.cs != nil {
		s.cs.SetSidecarWriteHook(func(path, reason string) {
			if path == "" {
				gs.EnqueueCommit(nil, reason)
				return
			}
			gs.EnqueueCommit([]string{path}, reason)
		})
	}

	st := gs.Status()
	return &st, nil
}

// gitConfigResponse is the shape returned by GET /api/git/config.
type gitConfigResponse struct {
	Enabled             bool   `json:"enabled"`
	Remote              string `json:"remote"`
	Branch              string `json:"branch"`
	AuthorName          string `json:"author_name"`
	AuthorEmail         string `json:"author_email"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	SSHKeyPath          string `json:"ssh_key_path"`
	HTTPSUsername       string `json:"https_username"`
	WebhookSecretSet    bool   `json:"webhook_secret_set"`
	HTTPSTokenSet       bool   `json:"https_token_set"`
	Source              string `json:"source"` // "db" or "yaml"

	// Behaviour toggles.
	PollEnabled      bool `json:"poll_enabled"`
	AutoPushEnabled  bool `json:"auto_push_enabled"`
	AutoApplyEnabled bool `json:"auto_apply_enabled"`
	WebhookEnabled   bool `json:"webhook_enabled"`
}

// gitConfigRequest is the shape accepted by PUT /api/git/config.
type gitConfigRequest struct {
	Enabled             bool    `json:"enabled"`
	Remote              string  `json:"remote"`
	Branch              string  `json:"branch"`
	AuthorName          string  `json:"author_name"`
	AuthorEmail         string  `json:"author_email"`
	PollIntervalSeconds int     `json:"poll_interval_seconds"`
	SSHKeyPath          string  `json:"ssh_key_path"`
	HTTPSUsername       string  `json:"https_username"`
	WebhookSecret       *string `json:"webhook_secret"` // nil=unchanged, ""=clear, "x"=set
	HTTPSToken          *string `json:"https_token"`    // same

	// Behaviour toggles (optional; absent=true for backwards-compat).
	PollEnabled      *bool `json:"poll_enabled"`
	AutoPushEnabled  *bool `json:"auto_push_enabled"`
	AutoApplyEnabled *bool `json:"auto_apply_enabled"`
	WebhookEnabled   *bool `json:"webhook_enabled"`
}

func (s *Server) handleGetGitConfig(w http.ResponseWriter, r *http.Request) {
	dbKV, err := s.store.GetGitSyncConfig()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	var resp gitConfigResponse
	if len(dbKV) > 0 {
		resp.Source = "db"
		resp.Enabled = dbKV["enabled"] == "true"
		resp.Remote = dbKV["remote"]
		resp.Branch = dbKV["branch"]
		resp.AuthorName = dbKV["author_name"]
		resp.AuthorEmail = dbKV["author_email"]
		resp.SSHKeyPath = dbKV["ssh_key_path"]
		resp.HTTPSUsername = dbKV["https_username"]
		resp.WebhookSecretSet = dbKV["webhook_secret_enc"] != ""
		resp.HTTPSTokenSet = dbKV["https_token_enc"] != ""
		if v, ok := dbKV["poll_interval"]; ok && v != "" {
			if secs, parseErr := strconv.Atoi(v); parseErr == nil {
				resp.PollIntervalSeconds = secs
			}
		}
		// Toggles: missing key defaults to true.
		resp.PollEnabled = dbKV["poll_enabled"] != "false"
		resp.AutoPushEnabled = dbKV["auto_push_enabled"] != "false"
		resp.AutoApplyEnabled = dbKV["auto_apply_enabled"] != "false"
		resp.WebhookEnabled = dbKV["webhook_enabled"] != "false"
	} else if s.cfg != nil && s.cfg.GitSync.Enabled {
		resp.Source = "yaml"
		resp.Enabled = s.cfg.GitSync.Enabled
		resp.Remote = s.cfg.GitSync.Remote
		resp.Branch = s.cfg.GitSync.Branch
		resp.AuthorName = s.cfg.GitSync.AuthorName
		resp.AuthorEmail = s.cfg.GitSync.AuthorEmail
		resp.SSHKeyPath = s.cfg.GitSync.SSHKeyPath
		resp.HTTPSUsername = s.cfg.GitSync.HTTPSUsername
		resp.WebhookSecretSet = s.cfg.GitSync.WebhookSecret != ""
		resp.HTTPSTokenSet = s.cfg.GitSync.HTTPSToken != ""
		resp.PollIntervalSeconds = int(s.cfg.GitSync.PollInterval.Seconds())
		// YAML path has no toggle fields; all default to true.
		resp.PollEnabled = true
		resp.AutoPushEnabled = true
		resp.AutoApplyEnabled = true
		resp.WebhookEnabled = true
	} else {
		resp.Source = "yaml"
		resp.Enabled = false
	}

	// Apply defaults for display.
	if resp.Branch == "" {
		resp.Branch = "main"
	}
	if resp.AuthorName == "" {
		resp.AuthorName = "SimpleDeploy"
	}
	if resp.AuthorEmail == "" {
		resp.AuthorEmail = "bot@simpledeploy.local"
	}
	if resp.PollIntervalSeconds == 0 {
		resp.PollIntervalSeconds = 60
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handlePutGitConfig(w http.ResponseWriter, r *http.Request) {
	var req gitConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validation.
	if req.Enabled && req.Remote == "" {
		http.Error(w, "remote is required when enabled", http.StatusBadRequest)
		return
	}
	if req.PollIntervalSeconds != 0 && req.PollIntervalSeconds < 5 {
		http.Error(w, "poll_interval_seconds must be >= 5", http.StatusBadRequest)
		return
	}

	// Build the new KV map.
	kvs := map[string]string{
		"enabled":            boolStr(req.Enabled),
		"remote":             req.Remote,
		"branch":             req.Branch,
		"author_name":        req.AuthorName,
		"author_email":       req.AuthorEmail,
		"ssh_key_path":       req.SSHKeyPath,
		"https_username":     req.HTTPSUsername,
		"poll_interval":      strconv.Itoa(req.PollIntervalSeconds),
		"poll_enabled":       boolPtrStr(req.PollEnabled, true),
		"auto_push_enabled":  boolPtrStr(req.AutoPushEnabled, true),
		"auto_apply_enabled": boolPtrStr(req.AutoApplyEnabled, true),
		"webhook_enabled":    boolPtrStr(req.WebhookEnabled, true),
	}

	// Fetch existing encrypted values to support "unchanged" semantics.
	existing, err := s.store.GetGitSyncConfig()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	// webhook_secret: nil=keep, ""=clear, non-empty=encrypt+store.
	if req.WebhookSecret == nil {
		kvs["webhook_secret_enc"] = existing["webhook_secret_enc"]
	} else if *req.WebhookSecret == "" {
		kvs["webhook_secret_enc"] = ""
	} else {
		enc, encErr := auth.Encrypt(*req.WebhookSecret, s.masterSecret)
		if encErr != nil {
			httpError(w, fmt.Errorf("encrypt webhook_secret: %w", encErr), http.StatusInternalServerError)
			return
		}
		kvs["webhook_secret_enc"] = enc
	}

	// https_token: same semantics.
	if req.HTTPSToken == nil {
		kvs["https_token_enc"] = existing["https_token_enc"]
	} else if *req.HTTPSToken == "" {
		kvs["https_token_enc"] = ""
	} else {
		enc, encErr := auth.Encrypt(*req.HTTPSToken, s.masterSecret)
		if encErr != nil {
			httpError(w, fmt.Errorf("encrypt https_token: %w", encErr), http.StatusInternalServerError)
			return
		}
		kvs["https_token_enc"] = enc
	}

	// Validate remote reachability+auth before persisting so the user gets
	// immediate feedback instead of silent push failures later.
	if req.Enabled {
		probe := gitsync.Config{
			Remote:        req.Remote,
			Branch:        req.Branch,
			SSHKeyPath:    req.SSHKeyPath,
			HTTPSUsername: req.HTTPSUsername,
		}
		switch {
		case req.HTTPSToken == nil:
			if existing["https_token_enc"] != "" {
				if dec, decErr := auth.Decrypt(existing["https_token_enc"], s.masterSecret); decErr == nil {
					probe.HTTPSToken = dec
				}
			}
		case *req.HTTPSToken != "":
			probe.HTTPSToken = *req.HTTPSToken
		}
		res := gitsync.CheckRemote(probe)
		if !res.OK {
			msg, ok := testConnMessages[res.Code]
			if !ok {
				msg = testConnMessages["unknown"]
			}
			http.Error(w, msg+" ("+res.RawError+")", http.StatusBadRequest)
			return
		}
	}

	if err := s.store.SetGitSyncConfig(kvs); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	st, err := s.ReloadGitSync(r.Context())
	if err != nil {
		httpError(w, fmt.Errorf("reload gitsync: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

func (s *Server) handleDisableGitSync(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGitSyncConfig(); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	st, err := s.ReloadGitSync(r.Context())
	if err != nil {
		httpError(w, fmt.Errorf("reload gitsync: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// boolPtrStr returns "true"/"false" from a *bool pointer.
// When ptr is nil, the def (default) value is used.
func boolPtrStr(ptr *bool, def bool) string {
	if ptr == nil {
		return boolStr(def)
	}
	return boolStr(*ptr)
}

// wrapGsMu wraps the existing git handlers to acquire a read lock on gsMu.
// This is a thin adapter so concurrent ReloadGitSync is safe.
func (s *Server) handleGitStatusSafe(w http.ResponseWriter, r *http.Request) {
	gsMu.RLock()
	defer gsMu.RUnlock()
	s.handleGitStatus(w, r)
}

func (s *Server) handleGitSyncNowSafe(w http.ResponseWriter, r *http.Request) {
	gsMu.RLock()
	defer gsMu.RUnlock()
	s.handleGitSyncNow(w, r)
}

func (s *Server) handleGitWebhookSafe(w http.ResponseWriter, r *http.Request) {
	gsMu.RLock()
	defer gsMu.RUnlock()
	s.handleGitWebhook(w, r)
}
