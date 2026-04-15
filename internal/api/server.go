package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/vazra/simpledeploy/internal/alerts"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/logbuf"
	"github.com/vazra/simpledeploy/internal/store"
)

// httpError logs the real error and returns a generic message to the client.
func httpError(w http.ResponseWriter, err error, code int) {
	log.Printf("[api] %s", err)
	msg := http.StatusText(code)
	if code == http.StatusNotFound {
		msg = "not found"
	}
	http.Error(w, msg, code)
}

type Server struct {
	mux             *http.ServeMux
	port            int
	store           *store.Store
	jwt             *auth.JWTManager
	rateLimiter     *auth.RateLimiter
	backupScheduler *backup.Scheduler
	docker          docker.Client
	appsDir         string
	reconciler      reconciler
	lockout         *auth.LoginLockout
	audit           *audit.Logger
	trustedProxies  []string
	masterSecret    string
	buildVersion    string
	buildCommit     string
	buildDate       string
	dbPath          string
	logBuf          *logbuf.Buffer
	dbBackupCron        *cron.Cron
	webhookDispatcher   *alerts.WebhookDispatcher
	startedAt           time.Time
	tlsMode             string
	dataDir             string
}

func NewServer(port int, st *store.Store, jwtMgr *auth.JWTManager, rl *auth.RateLimiter) *Server {
	s := &Server{
		mux:         http.NewServeMux(),
		port:        port,
		store:       st,
		jwt:         jwtMgr,
		rateLimiter: rl,
		startedAt:   time.Now(),
	}
	s.routes()
	return s
}

// SetMasterSecret sets the master secret for encrypting registry credentials.
func (s *Server) SetMasterSecret(secret string) { s.masterSecret = secret }

// SetBuildInfo sets the build version, commit, and date.
func (s *Server) SetBuildInfo(version, commit, date string) {
	s.buildVersion = version
	s.buildCommit = commit
	s.buildDate = date
}

// SetDBPath sets the path to the SQLite database file.
func (s *Server) SetDBPath(path string) { s.dbPath = path }

// SetBackupScheduler sets the backup scheduler (can be nil).
func (s *Server) SetBackupScheduler(sched *backup.Scheduler) {
	s.backupScheduler = sched
}

// SetLockout sets the login lockout tracker.
func (s *Server) SetLockout(l *auth.LoginLockout) { s.lockout = l }

// SetAudit sets the audit logger.
func (s *Server) SetAudit(a *audit.Logger) { s.audit = a }

// SetLogBuffer sets the log buffer for system log streaming.
func (s *Server) SetLogBuffer(lb *logbuf.Buffer) { s.logBuf = lb }

// SetWebhookDispatcher sets the webhook dispatcher for test webhook functionality.
func (s *Server) SetWebhookDispatcher(d *alerts.WebhookDispatcher) { s.webhookDispatcher = d }

// SetTrustedProxies sets the trusted proxy IPs for X-Forwarded-For parsing.
func (s *Server) SetTrustedProxies(proxies []string) { s.trustedProxies = proxies }

func (s *Server) SetTLSMode(mode string) { s.tlsMode = mode }
func (s *Server) SetDataDir(dir string)  { s.dataDir = dir }

// SetDocker sets the docker client.
func (s *Server) SetDocker(dc docker.Client) { s.docker = dc }

// SetUIFS serves the embedded SPA with fallback to index.html.
func (s *Server) SetUIFS(fsys fs.FS) {
	fileServer := http.FileServer(http.FS(fsys))
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			f, err := fsys.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback
		r.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, r)
	}))
}

func (s *Server) routes() {
	// Public routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /api/setup/status", s.handleSetupStatus)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)

	// Trust page (local TLS only, unauthenticated)
	s.mux.HandleFunc("GET /trust", s.handleTrustPage)
	s.mux.HandleFunc("GET /api/tls/ca.crt", s.handleCACert)

	// Protected routes
	s.mux.Handle("GET /api/apps", s.authMiddleware(
		http.HandlerFunc(s.handleListApps)))
	s.mux.Handle("GET /api/apps/{slug}", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleGetApp))))

	// User management (auth + super_admin)
	s.mux.Handle("GET /api/users", s.authMiddleware(http.HandlerFunc(s.handleListUsers)))
	s.mux.Handle("POST /api/users", s.authMiddleware(http.HandlerFunc(s.handleCreateUser)))
	s.mux.Handle("PUT /api/users/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateUser)))
	s.mux.Handle("DELETE /api/users/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteUser)))
	s.mux.Handle("POST /api/users/{id}/access", s.authMiddleware(http.HandlerFunc(s.handleGrantAccess)))
	s.mux.Handle("DELETE /api/users/{id}/access/{slug}", s.authMiddleware(http.HandlerFunc(s.handleRevokeAccess)))

	// Profile (auth)
	s.mux.Handle("GET /api/me", s.authMiddleware(http.HandlerFunc(s.handleGetMe)))
	s.mux.Handle("PUT /api/me", s.authMiddleware(http.HandlerFunc(s.handleUpdateMe)))
	s.mux.Handle("PUT /api/me/password", s.authMiddleware(http.HandlerFunc(s.handleChangePassword)))

	// API key management (auth)
	s.mux.Handle("GET /api/apikeys", s.authMiddleware(http.HandlerFunc(s.handleListAPIKeys)))
	s.mux.Handle("POST /api/apikeys", s.authMiddleware(http.HandlerFunc(s.handleCreateAPIKey)))
	s.mux.Handle("DELETE /api/apikeys/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteAPIKey)))

	// Metrics
	s.mux.Handle("GET /api/metrics/system", s.authMiddleware(http.HandlerFunc(s.handleSystemMetrics)))
	s.mux.Handle("GET /api/apps/{slug}/metrics", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleAppMetrics))))

	// Request stats
	s.mux.Handle("GET /api/apps/{slug}/requests", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleAppRequests))))

	// Logs (WebSocket)
	s.mux.Handle("GET /api/apps/{slug}/deploy-logs", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleDeployLogs))))
	s.mux.Handle("GET /api/apps/{slug}/logs", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleLogs))))

	// Deploy / remove / compose
	s.mux.Handle("POST /api/apps/deploy", s.authMiddleware(http.HandlerFunc(s.handleDeploy)))
	s.mux.Handle("POST /api/apps/validate-compose", s.authMiddleware(http.HandlerFunc(s.handleValidateCompose)))
	s.mux.Handle("DELETE /api/apps/{slug}", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleRemoveApp))))
	s.mux.Handle("GET /api/apps/{slug}/compose", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleGetCompose))))

	// App actions
	s.mux.Handle("POST /api/apps/{slug}/restart", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleRestart))))
	s.mux.Handle("POST /api/apps/{slug}/stop", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleStop))))
	s.mux.Handle("POST /api/apps/{slug}/start", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleStart))))
	s.mux.Handle("POST /api/apps/{slug}/pull", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handlePull))))
	s.mux.Handle("POST /api/apps/{slug}/scale", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleScale))))
	s.mux.Handle("GET /api/apps/{slug}/services", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleGetServices))))
	s.mux.Handle("GET /api/apps/{slug}/env", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleGetEnv))))
	s.mux.Handle("PUT /api/apps/{slug}/env", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handlePutEnv))))

	// Endpoints
	s.mux.Handle("PUT /api/apps/{slug}/endpoints", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleUpdateEndpoints))))

	// Certs
	s.mux.Handle("PUT /api/apps/{slug}/certs/{domain}", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleUploadCert))))
	s.mux.Handle("DELETE /api/apps/{slug}/certs/{domain}", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleDeleteCert))))

	// IP access
	s.mux.Handle("PUT /api/apps/{slug}/access", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleUpdateAccess))))

	// Cancel deploy
	s.mux.Handle("POST /api/apps/{slug}/cancel", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleCancel))))

	// Deploy history
	s.mux.Handle("GET /api/apps/{slug}/versions", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleListVersions))))
	s.mux.Handle("POST /api/apps/{slug}/rollback", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleRollback))))
	s.mux.Handle("DELETE /api/apps/{slug}/versions/{id}", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleDeleteVersion))))
	s.mux.Handle("GET /api/apps/{slug}/events", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleListDeployEvents))))

	// Webhooks
	s.mux.Handle("GET /api/webhooks", s.authMiddleware(http.HandlerFunc(s.handleListWebhooks)))
	s.mux.Handle("POST /api/webhooks", s.authMiddleware(http.HandlerFunc(s.handleCreateWebhook)))
	s.mux.Handle("PUT /api/webhooks/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateWebhook)))
	s.mux.Handle("DELETE /api/webhooks/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteWebhook)))
	s.mux.Handle("POST /api/webhooks/test", s.authMiddleware(http.HandlerFunc(s.handleTestWebhook)))

	// Alert rules
	s.mux.Handle("GET /api/alerts/rules", s.authMiddleware(http.HandlerFunc(s.handleListAlertRules)))
	s.mux.Handle("POST /api/alerts/rules", s.authMiddleware(http.HandlerFunc(s.handleCreateAlertRule)))
	s.mux.Handle("PUT /api/alerts/rules/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateAlertRule)))
	s.mux.Handle("DELETE /api/alerts/rules/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteAlertRule)))

	// Alert history
	s.mux.Handle("GET /api/alerts/history", s.authMiddleware(http.HandlerFunc(s.handleListAlertHistory)))
	s.mux.Handle("DELETE /api/alerts/history", s.authMiddleware(http.HandlerFunc(s.handleClearAlertHistory)))

	// Backup configs
	s.mux.Handle("GET /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleListBackupConfigs)))
	s.mux.Handle("POST /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleCreateBackupConfig)))
	s.mux.Handle("DELETE /api/backups/configs/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteBackupConfig)))

	// Backup runs
	s.mux.Handle("GET /api/apps/{slug}/backups/runs", s.authMiddleware(http.HandlerFunc(s.handleListBackupRuns)))
	s.mux.Handle("POST /api/apps/{slug}/backups/run", s.authMiddleware(http.HandlerFunc(s.handleTriggerBackup)))
	s.mux.Handle("POST /api/backups/restore/{id}", s.authMiddleware(http.HandlerFunc(s.handleRestore)))

	// Backup dashboard & detection
	s.mux.Handle("GET /api/backups/summary", s.authMiddleware(http.HandlerFunc(s.handleBackupSummary)))
	s.mux.Handle("GET /api/apps/{slug}/backups/detect", s.authMiddleware(http.HandlerFunc(s.handleDetectStrategies)))
	s.mux.Handle("POST /api/backups/configs/{id}/run", s.authMiddleware(http.HandlerFunc(s.handleTriggerBackupConfig)))
	s.mux.Handle("POST /api/backups/test-s3", s.authMiddleware(http.HandlerFunc(s.handleTestS3)))

	// Docker system management
	s.mux.Handle("GET /api/docker/info", s.authMiddleware(http.HandlerFunc(s.handleDockerInfo)))
	s.mux.Handle("GET /api/docker/disk-usage", s.authMiddleware(http.HandlerFunc(s.handleDockerDiskUsage)))
	s.mux.Handle("POST /api/docker/prune/containers", s.authMiddleware(http.HandlerFunc(s.handleDockerPruneContainers)))
	s.mux.Handle("POST /api/docker/prune/images", s.authMiddleware(http.HandlerFunc(s.handleDockerPruneImages)))
	s.mux.Handle("POST /api/docker/prune/volumes", s.authMiddleware(http.HandlerFunc(s.handleDockerPruneVolumes)))
	s.mux.Handle("POST /api/docker/prune/build-cache", s.authMiddleware(http.HandlerFunc(s.handleDockerPruneBuildCache)))
	s.mux.Handle("POST /api/docker/prune/all", s.authMiddleware(http.HandlerFunc(s.handleDockerPruneAll)))
	s.mux.Handle("GET /api/docker/images", s.authMiddleware(http.HandlerFunc(s.handleDockerImages)))
	s.mux.Handle("DELETE /api/docker/images/{id}", s.authMiddleware(http.HandlerFunc(s.handleDockerImageRemove)))
	s.mux.Handle("GET /api/docker/networks", s.authMiddleware(http.HandlerFunc(s.handleDockerNetworks)))
	s.mux.Handle("GET /api/docker/volumes", s.authMiddleware(http.HandlerFunc(s.handleDockerVolumes)))
	s.mux.Handle("DELETE /api/docker/networks/{id}", s.authMiddleware(http.HandlerFunc(s.handleDockerNetworkRemove)))
	s.mux.Handle("DELETE /api/docker/volumes/{name}", s.authMiddleware(http.HandlerFunc(s.handleDockerVolumeRemove)))

	// System management
	s.mux.Handle("GET /api/system/info", s.authMiddleware(http.HandlerFunc(s.handleSystemInfo)))
	s.mux.Handle("GET /api/system/storage-breakdown", s.authMiddleware(http.HandlerFunc(s.handleStorageBreakdown)))
	s.mux.Handle("POST /api/system/prune/metrics", s.authMiddleware(http.HandlerFunc(s.handlePruneMetrics)))
	s.mux.Handle("POST /api/system/prune/request-stats", s.authMiddleware(http.HandlerFunc(s.handlePruneRequestMetrics)))
	s.mux.Handle("POST /api/system/vacuum", s.authMiddleware(http.HandlerFunc(s.handleVacuumDB)))
	s.mux.Handle("GET /api/system/audit-log", s.authMiddleware(http.HandlerFunc(s.handleAuditLog)))
	s.mux.Handle("DELETE /api/system/audit-log", s.authMiddleware(http.HandlerFunc(s.handleClearAuditLog)))
	s.mux.Handle("GET /api/system/audit-config", s.authMiddleware(http.HandlerFunc(s.handleGetAuditConfig)))
	s.mux.Handle("PUT /api/system/audit-config", s.authMiddleware(http.HandlerFunc(s.handleUpdateAuditConfig)))

	// System logs
	s.mux.Handle("GET /api/system/process-logs", s.authMiddleware(http.HandlerFunc(s.handleSystemLogs)))
	s.mux.Handle("GET /api/system/process-logs/stream", s.authMiddleware(http.HandlerFunc(s.handleSystemLogsWS)))

	// DB backup
	s.mux.Handle("POST /api/system/backup/download", s.authMiddleware(http.HandlerFunc(s.handleDBBackupDownload)))
	s.mux.Handle("GET /api/system/backup/config", s.authMiddleware(http.HandlerFunc(s.handleGetDBBackupConfig)))
	s.mux.Handle("POST /api/system/backup/config", s.authMiddleware(http.HandlerFunc(s.handleSetDBBackupConfig)))
	s.mux.Handle("GET /api/system/backup/runs", s.authMiddleware(http.HandlerFunc(s.handleListDBBackupRuns)))

	// Registry management
	s.mux.Handle("GET /api/registries", s.authMiddleware(http.HandlerFunc(s.handleListRegistries)))
	s.mux.Handle("POST /api/registries", s.authMiddleware(http.HandlerFunc(s.handleCreateRegistry)))
	s.mux.Handle("PUT /api/registries/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateRegistry)))
	s.mux.Handle("DELETE /api/registries/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteRegistry)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

// securityHeaders adds standard security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) InitDBBackupSchedule() {
	cfg, err := s.store.GetDBBackupConfig()
	if err != nil || cfg["enabled"] != "true" {
		return
	}
	s.scheduleDBBackup(cfg)
}

func (s *Server) scheduleDBBackup(cfg map[string]string) {
	if s.dbBackupCron != nil {
		s.dbBackupCron.Stop()
	}
	schedule := cfg["schedule"]
	if schedule == "" {
		return
	}
	dest := cfg["destination"]
	if dest == "" {
		return
	}
	compact := cfg["compact"] == "true"
	retention := 7
	if r, err := strconv.Atoi(cfg["retention"]); err == nil && r > 0 {
		retention = r
	}

	c := cron.New()
	c.AddFunc(schedule, func() {
		s.runDBBackup(dest, compact, retention)
	})
	c.Start()
	s.dbBackupCron = c
}

func (s *Server) runDBBackup(destDir string, compact bool, retention int) {
	ts := time.Now().Format("20060102-150405")
	fname := fmt.Sprintf("simpledeploy-%s.db", ts)
	destPath := filepath.Join(destDir, fname)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("[db-backup] mkdir %s: %v", destDir, err)
		s.store.CreateDBBackupRun(destPath, 0, compact, "failed", err.Error())
		return
	}

	if err := s.store.VacuumInto(destPath); err != nil {
		log.Printf("[db-backup] vacuum into: %v", err)
		s.store.CreateDBBackupRun(destPath, 0, compact, "failed", err.Error())
		return
	}

	if compact {
		if err := stripMetrics(destPath); err != nil {
			log.Printf("[db-backup] strip metrics: %v", err)
			s.store.CreateDBBackupRun(destPath, 0, true, "failed", err.Error())
			os.Remove(destPath)
			return
		}
	}

	var size int64
	if fi, err := os.Stat(destPath); err == nil {
		size = fi.Size()
	}
	s.store.CreateDBBackupRun(destPath, size, compact, "ok", "")
	log.Printf("[db-backup] created %s (%d bytes, compact=%v)", destPath, size, compact)

	// Prune old backups
	paths, err := s.store.PruneDBBackupRuns(retention)
	if err != nil {
		log.Printf("[db-backup] prune records: %v", err)
		return
	}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			log.Printf("[db-backup] remove old file %s: %v", p, err)
		}
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: s.Handler(),
	}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}
