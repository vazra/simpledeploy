package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/store"
)

type systemInfoResponse struct {
	SimpleDeploy simpleDeployInfo `json:"simpledeploy"`
	Resources    systemResources  `json:"resources"`
	Database     databaseInfo     `json:"database"`
	Apps         appSummary       `json:"apps"`
}

type simpleDeployInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	Uptime    string `json:"uptime"`
	UptimeSec int64  `json:"uptime_sec"`
	GoVersion string `json:"go_version"`
}

type systemResources struct {
	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskAvail   uint64  `json:"disk_avail"`
	DiskUsedPct float64 `json:"disk_used_pct"`
	RAMTotal    uint64  `json:"ram_total"`
	RAMUsed     uint64  `json:"ram_used"`
	RAMAvail    uint64  `json:"ram_avail"`
	CPUCount    int     `json:"cpu_count"`
}

type databaseInfo struct {
	Path         string        `json:"path"`
	SizeBytes    int64         `json:"size_bytes"`
	MigrationVer int64         `json:"migration_version"`
	RowCounts    store.DBStats `json:"row_counts"`
}

type appSummary struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Stopped int `json:"stopped"`
	Error   int `json:"error"`
}

type pruneRequest struct {
	Days int    `json:"days"`
	Tier string `json:"tier"` // raw, 1m, 5m, 1h — empty defaults to "raw"
}
type pruneResponse struct {
	Deleted int64  `json:"deleted"`
	Message string `json:"message"`
}

type storageBreakdownResponse struct {
	Metrics      []store.TierStat `json:"metrics"`
	RequestStats []store.TierStat `json:"request_stats"`
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startedAt)

	sd := simpleDeployInfo{
		Version:   s.buildVersion,
		Commit:    s.buildCommit,
		BuildDate: s.buildDate,
		Uptime:    formatUptime(uptime),
		UptimeSec: int64(uptime.Seconds()),
		GoVersion: runtime.Version(),
	}

	var res systemResources
	res.CPUCount = runtime.NumCPU()
	dataDir := filepath.Dir(s.dbPath)
	if dataDir == "" {
		dataDir = "."
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dataDir, &stat); err == nil {
		res.DiskTotal = stat.Blocks * uint64(stat.Bsize)
		res.DiskAvail = stat.Bavail * uint64(stat.Bsize)
		res.DiskUsed = res.DiskTotal - (stat.Bfree * uint64(stat.Bsize))
		if res.DiskTotal > 0 {
			res.DiskUsedPct = float64(res.DiskUsed) / float64(res.DiskTotal) * 100
		}
	}
	res.RAMTotal, res.RAMUsed, res.RAMAvail = ramStats()

	var dbInfo databaseInfo
	dbInfo.Path = s.dbPath
	if fi, err := os.Stat(s.dbPath); err == nil {
		dbInfo.SizeBytes = fi.Size()
	}
	dbStats, _ := s.store.GetDBStats()
	dbInfo.MigrationVer = dbStats.MigrationVer
	dbInfo.RowCounts = dbStats

	apps, _ := s.store.ListApps()
	var summary appSummary
	summary.Total = len(apps)
	for _, a := range apps {
		switch a.Status {
		case "running":
			summary.Running++
		case "stopped":
			summary.Stopped++
		default:
			summary.Error++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(systemInfoResponse{
		SimpleDeploy: sd,
		Resources:    res,
		Database:     dbInfo,
		Apps:         summary,
	})
}

func (s *Server) handleStorageBreakdown(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.store.GetMetricsTierStats()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	rs, err := s.store.GetRequestStatsTierStats()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(storageBreakdownResponse{Metrics: metrics, RequestStats: rs})
}

func parsePruneRequest(r *http.Request) pruneRequest {
	var req pruneRequest
	json.NewDecoder(r.Body).Decode(&req)
	if req.Days <= 0 {
		req.Days = 30
	}
	validTiers := map[string]bool{"raw": true, "1m": true, "5m": true, "1h": true}
	if !validTiers[req.Tier] {
		req.Tier = "raw"
	}
	return req
}

func (s *Server) handlePruneMetrics(w http.ResponseWriter, r *http.Request) {
	req := parsePruneRequest(r)
	cutoff := time.Now().AddDate(0, 0, -req.Days)
	n, err := s.store.PruneMetrics(req.Tier, cutoff)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pruneResponse{Deleted: n, Message: fmt.Sprintf("Deleted %d metrics[%s] rows older than %d days", n, req.Tier, req.Days)})
}

func (s *Server) handlePruneRequestStats(w http.ResponseWriter, r *http.Request) {
	req := parsePruneRequest(r)
	cutoff := time.Now().AddDate(0, 0, -req.Days)
	n, err := s.store.PruneRequestStats(req.Tier, cutoff)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pruneResponse{Deleted: n, Message: fmt.Sprintf("Deleted %d request_stats[%s] rows older than %d days", n, req.Tier, req.Days)})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	var entries []audit.Event
	if s.audit != nil {
		entries = s.audit.Recent(limit)
	}
	if entries == nil {
		entries = []audit.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (s *Server) handleClearAuditLog(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	if s.audit != nil {
		s.audit.Clear()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateAuditConfig(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	var body struct {
		MaxSize int `json:"max_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.MaxSize < 10 || body.MaxSize > 10000 {
		http.Error(w, "max_size must be between 10 and 10000", http.StatusBadRequest)
		return
	}
	if s.audit != nil {
		s.audit.Resize(body.MaxSize)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGetAuditConfig(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	maxSize := 500
	if s.audit != nil {
		maxSize = s.audit.MaxSize()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"max_size": maxSize})
}

func (s *Server) handleVacuumDB(w http.ResponseWriter, r *http.Request) {
	if err := s.store.VacuumDB(); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	fi, _ := os.Stat(s.dbPath)
	var sizeBytes int64
	if fi != nil {
		sizeBytes = fi.Size()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message":    "VACUUM completed",
		"size_bytes": sizeBytes,
	})
}
