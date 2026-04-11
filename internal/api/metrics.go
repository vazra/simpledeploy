package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

type metricResponse struct {
	Timestamp time.Time `json:"timestamp"`
	CPUPct    float64   `json:"cpu_pct"`
	MemBytes  int64     `json:"mem_bytes"`
	MemLimit  int64     `json:"mem_limit"`
	NetRx     int64     `json:"net_rx"`
	NetTx     int64     `json:"net_tx"`
	DiskRead  int64     `json:"disk_read"`
	DiskWrite int64     `json:"disk_write"`
}

func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	defaultFrom := now.Add(-time.Hour)

	parseTime := func(s string) (time.Time, bool) {
		if s == "" {
			return time.Time{}, false
		}
		// try RFC3339
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.UTC(), true
		}
		// try unix timestamp
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(n, 0).UTC(), true
		}
		return time.Time{}, false
	}

	from, ok := parseTime(r.URL.Query().Get("from"))
	if !ok {
		from = defaultFrom
	}
	to, ok := parseTime(r.URL.Query().Get("to"))
	if !ok {
		to = now
	}
	return from, to
}

func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	from, to := parseTimeRange(r)
	tier := store.SelectTier(to.Sub(from))
	points, err := s.store.QueryMetrics(nil, tier, from, to)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	resp := make([]metricResponse, 0, len(points))
	for _, p := range points {
		resp = append(resp, metricResponse{
			Timestamp: p.Timestamp,
			CPUPct:    p.CPUPct,
			MemBytes:  p.MemBytes,
			MemLimit:  p.MemLimit,
			NetRx:     p.NetRx,
			NetTx:     p.NetTx,
			DiskRead:  p.DiskRead,
			DiskWrite: p.DiskWrite,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAppMetrics(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	from, to := parseTimeRange(r)
	tier := store.SelectTier(to.Sub(from))
	points, err := s.store.QueryMetrics(&app.ID, tier, from, to)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	resp := make([]metricResponse, 0, len(points))
	for _, p := range points {
		resp = append(resp, metricResponse{
			Timestamp: p.Timestamp,
			CPUPct:    p.CPUPct,
			MemBytes:  p.MemBytes,
			MemLimit:  p.MemLimit,
			NetRx:     p.NetRx,
			NetTx:     p.NetTx,
			DiskRead:  p.DiskRead,
			DiskWrite: p.DiskWrite,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
