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

func TestAppRequestMetricsEndpoint(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "myapp", Slug: "myapp", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	now := time.Now().Unix()
	points := []metrics.RequestMetricPoint{
		{AppID: app.ID, Ts: now - 30*60, Tier: "raw", Count: 100, ErrorCount: 5, AvgLatency: 12.5, MaxLatency: 95.0},
		{AppID: app.ID, Ts: now - 20*60, Tier: "raw", Count: 80, ErrorCount: 2, AvgLatency: 8.0, MaxLatency: 45.0},
		{AppID: app.ID, Ts: now - 10*60, Tier: "raw", Count: 120, ErrorCount: 10, AvgLatency: 15.0, MaxLatency: 200.0},
	}
	if err := st.InsertRequestMetrics(points); err != nil {
		t.Fatalf("insert request metrics: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/requests?range=1h", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var env requestMetricsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Interval <= 0 {
		t.Fatalf("interval = %d, want > 0", env.Interval)
	}
	// 3 data points + 2 gap markers (10min gaps >> 10s*1.5 threshold)
	if len(env.Points) != 5 {
		t.Fatalf("got %d points, want 5 (3 data + 2 gaps)", len(env.Points))
	}
	// first real point
	if env.Points[0].N == nil || *env.Points[0].N != 100 {
		t.Errorf("first point count = %v, want 100", env.Points[0].N)
	}
	if env.Points[0].E == nil || *env.Points[0].E != 5 {
		t.Errorf("first point errors = %v, want 5", env.Points[0].E)
	}
	// second point should be a gap marker (nil fields)
	if env.Points[1].N != nil {
		t.Errorf("gap marker should have nil N")
	}
}

func TestAppRequestMetricsNotFound(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodGet, "/api/apps/nonexistent/requests", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestAppRequestMetricsRequiresAuth(t *testing.T) {
	srv, _, _ := setupUserTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/requests", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAppRequestMetricsEmpty(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "empty", Slug: "empty", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/requests?range=1h", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env requestMetricsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(env.Points) != 0 {
		t.Errorf("points = %d, want 0", len(env.Points))
	}
}
