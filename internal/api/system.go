package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

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

type pruneRequest struct{ Days int `json:"days"` }
type pruneResponse struct {
	Deleted int64  `json:"deleted"`
	Message string `json:"message"`
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

func (s *Server) handlePruneMetrics(w http.ResponseWriter, r *http.Request) {
	var req pruneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Days <= 0 {
		req.Days = 30
	}
	cutoff := time.Now().AddDate(0, 0, -req.Days)
	n, err := s.store.PruneMetrics("raw", cutoff)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pruneResponse{Deleted: n, Message: fmt.Sprintf("Deleted %d raw metric rows older than %d days", n, req.Days)})
}

func (s *Server) handlePruneRequestStats(w http.ResponseWriter, r *http.Request) {
	var req pruneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Days <= 0 {
		req.Days = 30
	}
	cutoff := time.Now().AddDate(0, 0, -req.Days)
	n, err := s.store.PruneRequestStats("raw", cutoff)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pruneResponse{Deleted: n, Message: fmt.Sprintf("Deleted %d raw request stat rows older than %d days", n, req.Days)})
}

func (s *Server) handleVacuumDB(w http.ResponseWriter, r *http.Request) {
	if err := s.store.VacuumDB(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
