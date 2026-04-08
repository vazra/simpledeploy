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

	now := time.Now().UTC().Truncate(time.Second)
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
			Timestamp: now.Add(-30 * time.Minute),
			Tier:      metrics.TierRaw,
		},
	}
	if err := st.InsertMetrics(pts); err != nil {
		t.Fatalf("insert metrics: %v", err)
	}

	req := authedRequest(t, http.MethodGet, "/api/metrics/system", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp []metricResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("got %d points, want 1", len(resp))
	}
	if resp[0].CPUPct != 12.5 {
		t.Errorf("cpu_pct = %v, want 12.5", resp[0].CPUPct)
	}
	if resp[0].MemBytes != 1048576 {
		t.Errorf("mem_bytes = %v, want 1048576", resp[0].MemBytes)
	}
	if resp[0].NetRx != 1024 {
		t.Errorf("net_rx = %v, want 1024", resp[0].NetRx)
	}
}

func TestAppMetricsEndpoint(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "myapp", Slug: "myapp", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	appID := app.ID
	pts := []metrics.MetricPoint{
		{
			AppID:     &appID,
			CPUPct:    55.0,
			MemBytes:  2097152,
			MemLimit:  4194304,
			NetRx:     2048,
			NetTx:     1024,
			DiskRead:  100,
			DiskWrite: 200,
			Timestamp: now.Add(-20 * time.Minute),
			Tier:      metrics.TierRaw,
		},
	}
	if err := st.InsertMetrics(pts); err != nil {
		t.Fatalf("insert metrics: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/metrics", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp []metricResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("got %d points, want 1", len(resp))
	}
	if resp[0].CPUPct != 55.0 {
		t.Errorf("cpu_pct = %v, want 55.0", resp[0].CPUPct)
	}
	if resp[0].DiskRead != 100 {
		t.Errorf("disk_read = %v, want 100", resp[0].DiskRead)
	}
	if resp[0].DiskWrite != 200 {
		t.Errorf("disk_write = %v, want 200", resp[0].DiskWrite)
	}
}

func TestMetricsDefaultTimeRange(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	// point inside last hour - should be returned
	now := time.Now().UTC().Truncate(time.Second)
	insidePt := []metrics.MetricPoint{
		{
			CPUPct:    10.0,
			Timestamp: now.Add(-30 * time.Minute),
			Tier:      metrics.TierRaw,
		},
	}
	// point outside last hour - should not be returned
	outsidePt := []metrics.MetricPoint{
		{
			CPUPct:    99.0,
			Timestamp: now.Add(-2 * time.Hour),
			Tier:      metrics.TierRaw,
		},
	}
	if err := st.InsertMetrics(insidePt); err != nil {
		t.Fatalf("insert inside: %v", err)
	}
	if err := st.InsertMetrics(outsidePt); err != nil {
		t.Fatalf("insert outside: %v", err)
	}

	// no from/to params - defaults to last hour
	req := authedRequest(t, http.MethodGet, "/api/metrics/system", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp []metricResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("got %d points, want 1 (only last-hour point)", len(resp))
	}
	if resp[0].CPUPct != 10.0 {
		t.Errorf("cpu_pct = %v, want 10.0", resp[0].CPUPct)
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
