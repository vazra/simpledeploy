package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/store"
)

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
}

func NewServer(port int, st *store.Store, jwtMgr *auth.JWTManager, rl *auth.RateLimiter) *Server {
	s := &Server{
		mux:         http.NewServeMux(),
		port:        port,
		store:       st,
		jwt:         jwtMgr,
		rateLimiter: rl,
	}
	s.routes()
	return s
}

// SetBackupScheduler sets the backup scheduler (can be nil).
func (s *Server) SetBackupScheduler(sched *backup.Scheduler) {
	s.backupScheduler = sched
}

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
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)

	// Protected routes
	s.mux.Handle("GET /api/apps", s.authMiddleware(
		http.HandlerFunc(s.handleListApps)))
	s.mux.Handle("GET /api/apps/{slug}", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleGetApp))))

	// User management (auth + super_admin)
	s.mux.Handle("GET /api/users", s.authMiddleware(http.HandlerFunc(s.handleListUsers)))
	s.mux.Handle("POST /api/users", s.authMiddleware(http.HandlerFunc(s.handleCreateUser)))
	s.mux.Handle("DELETE /api/users/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteUser)))
	s.mux.Handle("POST /api/users/{id}/access", s.authMiddleware(http.HandlerFunc(s.handleGrantAccess)))
	s.mux.Handle("DELETE /api/users/{id}/access/{slug}", s.authMiddleware(http.HandlerFunc(s.handleRevokeAccess)))

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
	s.mux.Handle("GET /api/apps/{slug}/logs", s.authMiddleware(
		s.appAccessMiddleware(http.HandlerFunc(s.handleLogs))))

	// Deploy / remove / compose
	s.mux.Handle("POST /api/apps/deploy", s.authMiddleware(http.HandlerFunc(s.handleDeploy)))
	s.mux.Handle("POST /api/apps/validate-compose", s.authMiddleware(http.HandlerFunc(s.handleValidateCompose)))
	s.mux.Handle("DELETE /api/apps/{slug}", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleRemoveApp))))
	s.mux.Handle("GET /api/apps/{slug}/compose", s.authMiddleware(s.appAccessMiddleware(http.HandlerFunc(s.handleGetCompose))))

	// Webhooks
	s.mux.Handle("GET /api/webhooks", s.authMiddleware(http.HandlerFunc(s.handleListWebhooks)))
	s.mux.Handle("POST /api/webhooks", s.authMiddleware(http.HandlerFunc(s.handleCreateWebhook)))
	s.mux.Handle("DELETE /api/webhooks/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteWebhook)))

	// Alert rules
	s.mux.Handle("GET /api/alerts/rules", s.authMiddleware(http.HandlerFunc(s.handleListAlertRules)))
	s.mux.Handle("POST /api/alerts/rules", s.authMiddleware(http.HandlerFunc(s.handleCreateAlertRule)))
	s.mux.Handle("PUT /api/alerts/rules/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateAlertRule)))
	s.mux.Handle("DELETE /api/alerts/rules/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteAlertRule)))

	// Alert history
	s.mux.Handle("GET /api/alerts/history", s.authMiddleware(http.HandlerFunc(s.handleListAlertHistory)))

	// Backup configs
	s.mux.Handle("GET /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleListBackupConfigs)))
	s.mux.Handle("POST /api/apps/{slug}/backups/configs", s.authMiddleware(http.HandlerFunc(s.handleCreateBackupConfig)))
	s.mux.Handle("DELETE /api/backups/configs/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteBackupConfig)))

	// Backup runs
	s.mux.Handle("GET /api/apps/{slug}/backups/runs", s.authMiddleware(http.HandlerFunc(s.handleListBackupRuns)))
	s.mux.Handle("POST /api/apps/{slug}/backups/run", s.authMiddleware(http.HandlerFunc(s.handleTriggerBackup)))
	s.mux.Handle("POST /api/backups/restore/{id}", s.authMiddleware(http.HandlerFunc(s.handleRestore)))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	return http.ListenAndServe(addr, s.mux)
}
