package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
	"github.com/vazra/simpledeploy/internal/store"
)

func TestSystemMetricsEndpoint(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	now := time.Now().Unix()
	pts := []metrics.MetricPoint{
		{
			AppID:     nil,
			CPUPct:    12.5,
			MemBytes:  1048576,
			MemLimit:  4194304,
			NetRx:     1024,
			NetTx:     512,
			DiskRead:  0,
			DiskWrite: 0,
			Ts:        now - 30*60,
			Tier:      metrics.TierRaw,
		},
	}
	if err := st.InsertMetrics(pts); err != nil {
		t.Fatalf("insert metrics: %v", err)
	}

	req := authedRequest(t, http.MethodGet, "/api/metrics/system?range=1h", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var env metricsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Interval <= 0 {
		t.Fatalf("interval = %d, want > 0", env.Interval)
	}
	// system metrics have ContainerID=""
	cm, ok := env.Containers[""]
	if !ok {
		t.Fatalf("no container '' in response, got keys: %v", keysOf(env.Containers))
	}
	if len(cm.Points) != 1 {
		t.Fatalf("got %d points, want 1", len(cm.Points))
	}
	if cm.Points[0].C == nil || *cm.Points[0].C != 12.5 {
		t.Errorf("cpu = %v, want 12.5", cm.Points[0].C)
	}
	if cm.Points[0].M == nil || *cm.Points[0].M != 1048576 {
		t.Errorf("mem = %v, want 1048576", cm.Points[0].M)
	}
}

func TestAppMetricsEndpoint(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "myapp", Slug: "myapp", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	now := time.Now().Unix()
	appID := app.ID
	pts := []metrics.MetricPoint{
		{
			AppID:       &appID,
			ContainerID: "abc123",
			CPUPct:      55.0,
			MemBytes:    2097152,
			MemLimit:    4194304,
			NetRx:       2048,
			NetTx:       1024,
			DiskRead:    100,
			DiskWrite:   200,
			Ts:          now - 20*60,
			Tier:        metrics.TierRaw,
		},
	}
	if err := st.InsertMetrics(pts); err != nil {
		t.Fatalf("insert metrics: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/metrics?range=1h", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var env metricsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cm, ok := env.Containers["abc123"]
	if !ok {
		t.Fatalf("no container 'abc123' in response")
	}
	if len(cm.Points) != 1 {
		t.Fatalf("got %d points, want 1", len(cm.Points))
	}
	if cm.Points[0].C == nil || *cm.Points[0].C != 55.0 {
		t.Errorf("cpu = %v, want 55.0", cm.Points[0].C)
	}
	if cm.Points[0].DR == nil || *cm.Points[0].DR != 100 {
		t.Errorf("disk_read = %v, want 100", cm.Points[0].DR)
	}
}

func TestMetricsDefaultRange(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	now := time.Now().Unix()
	// inside last hour
	insidePt := []metrics.MetricPoint{
		{CPUPct: 10.0, Ts: now - 30*60, Tier: metrics.TierRaw},
	}
	// outside last hour
	outsidePt := []metrics.MetricPoint{
		{CPUPct: 99.0, Ts: now - 2*3600, Tier: metrics.TierRaw},
	}
	if err := st.InsertMetrics(insidePt); err != nil {
		t.Fatalf("insert inside: %v", err)
	}
	if err := st.InsertMetrics(outsidePt); err != nil {
		t.Fatalf("insert outside: %v", err)
	}

	// no range param defaults to 1h
	req := authedRequest(t, http.MethodGet, "/api/metrics/system", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var env metricsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	cm := env.Containers[""]
	if len(cm.Points) != 1 {
		t.Fatalf("got %d points, want 1 (only last-hour point)", len(cm.Points))
	}
	if cm.Points[0].C == nil || *cm.Points[0].C != 10.0 {
		t.Errorf("cpu = %v, want 10.0", cm.Points[0].C)
	}
}

func TestGapInsertion(t *testing.T) {
	points := []compactPoint{
		{T: 100},
		{T: 110},
		{T: 200}, // gap: 90 > 15 (10*1.5)
		{T: 210},
	}
	result := insertGaps(points, 10)
	// expect gap marker between T=110 and T=200
	if len(result) != 5 {
		t.Fatalf("got %d points, want 5 (4 + 1 gap)", len(result))
	}
	// gap marker at index 2
	if result[2].T != 120 {
		t.Errorf("gap marker T = %d, want 120", result[2].T)
	}
	if result[2].C != nil {
		t.Errorf("gap marker should have nil C")
	}
}

func TestAppMetricsNotFound(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodGet, "/api/apps/nonexistent/metrics", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestSystemMetricsRequiresAuth(t *testing.T) {
	srv, _, _ := setupUserTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/system", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func keysOf(m map[string]containerMetrics) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
