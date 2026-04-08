package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

type requestStatsResponse struct {
	TotalRequests int                `json:"total_requests"`
	AvgLatencyMs  float64            `json:"avg_latency_ms"`
	StatusCodes   map[string]int     `json:"status_codes"`
	Points        []requestStatPoint `json:"points"`
}

type requestStatPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	StatusCode  int       `json:"status_code"`
	LatencyMs   float64   `json:"latency_ms"`
	Method      string    `json:"method"`
	PathPattern string    `json:"path_pattern"`
}

func (s *Server) handleAppRequests(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	from, to := parseTimeRange(r)
	tier := store.SelectTier(to.Sub(from))
	stats, err := s.store.QueryRequestStats(app.ID, tier, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := buildRequestStatsResponse(stats)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func buildRequestStatsResponse(stats []store.RequestStat) requestStatsResponse {
	resp := requestStatsResponse{
		StatusCodes: make(map[string]int),
		Points:      make([]requestStatPoint, 0, len(stats)),
	}

	var totalLatency float64
	for _, st := range stats {
		resp.TotalRequests++
		totalLatency += st.LatencyMs

		bucket := statusCodeBucket(st.StatusCode)
		resp.StatusCodes[bucket]++

		resp.Points = append(resp.Points, requestStatPoint{
			Timestamp:   st.Timestamp,
			StatusCode:  st.StatusCode,
			LatencyMs:   st.LatencyMs,
			Method:      st.Method,
			PathPattern: st.PathPattern,
		})
	}

	if resp.TotalRequests > 0 {
		resp.AvgLatencyMs = totalLatency / float64(resp.TotalRequests)
	}

	return resp
}

func statusCodeBucket(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 200:
		return "2xx"
	default:
		return fmt.Sprintf("%dxx", code/100)
	}
}
