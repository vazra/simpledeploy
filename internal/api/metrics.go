package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/vazra/simpledeploy/internal/metrics"
)

type metricsEnvelope struct {
	Interval   int                         `json:"interval"`
	Containers map[string]containerMetrics `json:"containers"`
}

type containerMetrics struct {
	Points []compactPoint `json:"points"`
}

type compactPoint struct {
	T  int64    `json:"t"`
	C  *float64 `json:"c,omitempty"`
	M  *int64   `json:"m,omitempty"`
	ML *int64   `json:"ml,omitempty"`
	NR *float64 `json:"nr,omitempty"`
	NT *float64 `json:"nt,omitempty"`
	DR *float64 `json:"dr,omitempty"`
	DW *float64 `json:"dw,omitempty"`
}

func parseRange(r *http.Request) string {
	rng := r.URL.Query().Get("range")
	switch rng {
	case "1h", "6h", "24h", "1w", "1m", "1yr":
		return rng
	default:
		return "1h"
	}
}

func metricToCompact(p metrics.MetricPoint) compactPoint {
	return compactPoint{
		T:  p.Ts,
		C:  &p.CPUPct,
		M:  &p.MemBytes,
		ML: &p.MemLimit,
		NR: &p.NetRx,
		NT: &p.NetTx,
		DR: &p.DiskRead,
		DW: &p.DiskWrite,
	}
}

func insertGaps(points []compactPoint, intervalSec int) []compactPoint {
	if len(points) < 2 {
		return points
	}
	// Threshold for "missing data" gaps. Tier interval is the display granularity,
	// not the collection cadence: real collection may be slower (e.g. 30-60s) than
	// the tier interval (10s for raw). Using 1.5*tier would flag every normal
	// sample as a gap and break the line. Use the observed median spacing instead,
	// floored at 1.5*tier so we still cover the dense case.
	gaps := make([]int64, 0, len(points)-1)
	for i := 1; i < len(points); i++ {
		gaps = append(gaps, points[i].T-points[i-1].T)
	}
	sorted := append([]int64(nil), gaps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	median := sorted[len(sorted)/2]
	threshold := int64(float64(median) * 2)
	if min := int64(float64(intervalSec) * 1.5); threshold < min {
		threshold = min
	}
	result := []compactPoint{points[0]}
	for i := 1; i < len(points); i++ {
		if gaps[i-1] > threshold {
			result = append(result, compactPoint{T: points[i-1].T + median})
		}
		result = append(result, points[i])
	}
	return result
}

func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	rangeStr := parseRange(r)
	points, intervalSec, err := s.store.QueryMetrics(nil, rangeStr)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	env := buildMetricsEnvelope(points, intervalSec)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}

func (s *Server) handleAppMetrics(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rangeStr := parseRange(r)
	points, intervalSec, err := s.store.QueryMetrics(&app.ID, rangeStr)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	env := buildMetricsEnvelope(points, intervalSec)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}

func buildMetricsEnvelope(points []metrics.MetricPoint, intervalSec int) metricsEnvelope {
	grouped := make(map[string][]compactPoint)
	for _, p := range points {
		cp := metricToCompact(p)
		grouped[p.ContainerID] = append(grouped[p.ContainerID], cp)
	}

	containers := make(map[string]containerMetrics, len(grouped))
	for cid, pts := range grouped {
		containers[cid] = containerMetrics{Points: insertGaps(pts, intervalSec)}
	}

	return metricsEnvelope{
		Interval:   intervalSec,
		Containers: containers,
	}
}
