package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/metrics"
)

type requestMetricsEnvelope struct {
	Interval int               `json:"interval"`
	Points   []compactReqPoint `json:"points"`
}

type compactReqPoint struct {
	T  int64    `json:"t"`
	N  *int64   `json:"n,omitempty"`
	E  *int64   `json:"e,omitempty"`
	AL *float64 `json:"al,omitempty"`
	ML *float64 `json:"ml,omitempty"`
}

func reqMetricToCompact(p metrics.RequestMetricPoint) compactReqPoint {
	return compactReqPoint{
		T:  p.Ts,
		N:  &p.Count,
		E:  &p.ErrorCount,
		AL: &p.AvgLatency,
		ML: &p.MaxLatency,
	}
}

func insertReqGaps(points []compactReqPoint, intervalSec int) []compactReqPoint {
	if len(points) < 2 {
		return points
	}
	threshold := int64(float64(intervalSec) * 1.5)
	result := []compactReqPoint{points[0]}
	for i := 1; i < len(points); i++ {
		gap := points[i].T - points[i-1].T
		if gap > threshold {
			result = append(result, compactReqPoint{T: points[i-1].T + int64(intervalSec)})
		}
		result = append(result, points[i])
	}
	return result
}

func (s *Server) handleAppRequests(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	rangeStr := parseRange(r)
	points, intervalSec, err := s.store.QueryRequestMetrics(app.ID, rangeStr)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	compact := make([]compactReqPoint, 0, len(points))
	for _, p := range points {
		compact = append(compact, reqMetricToCompact(p))
	}

	env := requestMetricsEnvelope{
		Interval: intervalSec,
		Points:   insertReqGaps(compact, intervalSec),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(env)
}
