package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

func TestAppRequestsEndpoint(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "myapp", Slug: "myapp", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	stats := []store.RequestStat{
		{
			AppID:       app.ID,
			Timestamp:   now.Add(-30 * time.Minute),
			StatusCode:  200,
			LatencyMs:   10.0,
			Method:      "GET",
			PathPattern: "/users",
			Tier:        "raw",
		},
		{
			AppID:       app.ID,
			Timestamp:   now.Add(-20 * time.Minute),
			StatusCode:  404,
			LatencyMs:   5.0,
			Method:      "GET",
			PathPattern: "/notfound",
			Tier:        "raw",
		},
		{
			AppID:       app.ID,
			Timestamp:   now.Add(-10 * time.Minute),
			StatusCode:  500,
			LatencyMs:   50.0,
			Method:      "POST",
			PathPattern: "/orders",
			Tier:        "raw",
		},
	}
	if err := st.InsertRequestStats(stats); err != nil {
		t.Fatalf("insert request stats: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/requests", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp requestStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TotalRequests != 3 {
		t.Errorf("total_requests = %d, want 3", resp.TotalRequests)
	}

	wantAvg := (10.0 + 5.0 + 50.0) / 3.0
	if resp.AvgLatencyMs != wantAvg {
		t.Errorf("avg_latency_ms = %v, want %v", resp.AvgLatencyMs, wantAvg)
	}

	if resp.StatusCodes["2xx"] != 1 {
		t.Errorf("status_codes[2xx] = %d, want 1", resp.StatusCodes["2xx"])
	}
	if resp.StatusCodes["4xx"] != 1 {
		t.Errorf("status_codes[4xx] = %d, want 1", resp.StatusCodes["4xx"])
	}
	if resp.StatusCodes["5xx"] != 1 {
		t.Errorf("status_codes[5xx] = %d, want 1", resp.StatusCodes["5xx"])
	}

	if len(resp.Points) != 3 {
		t.Fatalf("points count = %d, want 3", len(resp.Points))
	}
}

func TestAppRequestsNotFound(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodGet, "/api/apps/nonexistent/requests", nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestAppRequestsRequiresAuth(t *testing.T) {
	srv, _, _ := setupUserTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/apps/myapp/requests", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAppRequestsEmptyResponse(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	app := &store.App{Name: "empty", Slug: "empty", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	req := authedRequest(t, http.MethodGet, fmt.Sprintf("/api/apps/%s/requests", app.Slug), nil, cookie)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp requestStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TotalRequests != 0 {
		t.Errorf("total_requests = %d, want 0", resp.TotalRequests)
	}
	if resp.AvgLatencyMs != 0 {
		t.Errorf("avg_latency_ms = %v, want 0", resp.AvgLatencyMs)
	}
	if len(resp.Points) != 0 {
		t.Errorf("points = %d, want 0", len(resp.Points))
	}
}
