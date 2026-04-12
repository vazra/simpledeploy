package alerts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/metrics"
	"github.com/vazra/simpledeploy/internal/store"
)

// --- mocks ---

type mockAlertStore struct {
	rules         []store.AlertRule
	activeAlerts  map[int64]*store.AlertHistory
	histories     []*store.AlertHistory
	webhooks      map[int64]*store.Webhook
	nextHistoryID int64
	resolvedIDs   []int64
}

func newMockAlertStore() *mockAlertStore {
	return &mockAlertStore{
		activeAlerts:  make(map[int64]*store.AlertHistory),
		webhooks:      make(map[int64]*store.Webhook),
		nextHistoryID: 1,
	}
}

func (m *mockAlertStore) ListActiveAlertRules() ([]store.AlertRule, error) {
	return m.rules, nil
}

func (m *mockAlertStore) GetActiveAlert(ruleID int64) (*store.AlertHistory, error) {
	return m.activeAlerts[ruleID], nil
}

func (m *mockAlertStore) CreateAlertHistory(ruleID int64, value float64, rule *store.AlertRule) (*store.AlertHistory, error) {
	h := &store.AlertHistory{
		ID:      m.nextHistoryID,
		RuleID:  ruleID,
		FiredAt: time.Now(),
		Value:   value,
	}
	m.nextHistoryID++
	m.histories = append(m.histories, h)
	m.activeAlerts[ruleID] = h
	return h, nil
}

func (m *mockAlertStore) ResolveAlert(historyID int64) error {
	m.resolvedIDs = append(m.resolvedIDs, historyID)
	for ruleID, h := range m.activeAlerts {
		if h.ID == historyID {
			delete(m.activeAlerts, ruleID)
			break
		}
	}
	return nil
}

func (m *mockAlertStore) GetWebhook(id int64) (*store.Webhook, error) {
	wh, ok := m.webhooks[id]
	if !ok {
		return &store.Webhook{ID: id, Type: "custom", URL: "http://localhost"}, nil
	}
	return wh, nil
}

type mockMetricQuerier struct {
	points []metrics.MetricPoint
}

func (m *mockMetricQuerier) QueryMetrics(appID *int64, rangeStr string) ([]metrics.MetricPoint, int, error) {
	return m.points, 10, nil
}

type mockAppLookup struct {
	apps map[int64]*store.App
}

func (m *mockAppLookup) GetAppByID(id int64) (*store.App, error) {
	if a, ok := m.apps[id]; ok {
		return a, nil
	}
	return &store.App{ID: id, Name: "testapp", Slug: "testapp"}, nil
}

// --- tests ---

func TestCheckCondition(t *testing.T) {
	tests := []struct {
		value, threshold float64
		op               string
		want             bool
	}{
		{95, 80, ">", true},
		{70, 80, ">", false},
		{80, 80, ">", false},
		{70, 80, "<", true},
		{95, 80, "<", false},
		{80, 80, "<", false},
		{80, 80, ">=", true},
		{81, 80, ">=", true},
		{79, 80, ">=", false},
		{80, 80, "<=", true},
		{79, 80, "<=", true},
		{81, 80, "<=", false},
		{80, 80, "??", false},
	}
	for _, tc := range tests {
		got := checkCondition(tc.value, tc.op, tc.threshold)
		if got != tc.want {
			t.Errorf("checkCondition(%v, %q, %v) = %v, want %v", tc.value, tc.op, tc.threshold, got, tc.want)
		}
	}
}

func TestExtractMetricValue(t *testing.T) {
	pt := metrics.MetricPoint{
		CPUPct:   42.5,
		MemBytes: 512 * 1024 * 1024,
		MemLimit: 1024 * 1024 * 1024,
	}

	if v := extractMetricValue(pt, "cpu_pct"); v != 42.5 {
		t.Errorf("cpu_pct = %v, want 42.5", v)
	}
	if v := extractMetricValue(pt, "mem_bytes"); v != float64(pt.MemBytes) {
		t.Errorf("mem_bytes = %v, want %v", v, float64(pt.MemBytes))
	}
	// 512MB / 1024MB * 100 = 50.0
	if v := extractMetricValue(pt, "mem_pct"); v != 50.0 {
		t.Errorf("mem_pct = %v, want 50.0", v)
	}

	// zero limit => 0
	pt2 := metrics.MetricPoint{MemBytes: 100, MemLimit: 0}
	if v := extractMetricValue(pt2, "mem_pct"); v != 0 {
		t.Errorf("mem_pct with zero limit = %v, want 0", v)
	}

	if v := extractMetricValue(pt, "unknown"); v != 0 {
		t.Errorf("unknown metric = %v, want 0", v)
	}
}

func TestEvaluateOnce_FiresAlert(t *testing.T) {
	var webhookHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		atomic.AddInt32(&webhookHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := newMockAlertStore()
	ms.rules = []store.AlertRule{
		{ID: 1, Metric: "cpu_pct", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: 10, Enabled: true},
	}
	ms.webhooks[10] = &store.Webhook{ID: 10, Type: "slack", URL: srv.URL}

	now := time.Now().Unix()
	mq := &mockMetricQuerier{
		points: []metrics.MetricPoint{
			{CPUPct: 90, Ts: now - 10},
			{CPUPct: 95, Ts: now - 20},
			{CPUPct: 85, Ts: now - 30},
		},
	}
	al := &mockAppLookup{apps: make(map[int64]*store.App)}

	e := NewEvaluator(ms, al, mq, NewWebhookDispatcherAllowPrivate())
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce: %v", err)
	}

	if atomic.LoadInt32(&webhookHits) != 1 {
		t.Errorf("webhook hits = %d, want 1", webhookHits)
	}
	if len(ms.histories) != 1 {
		t.Errorf("histories = %d, want 1", len(ms.histories))
	}
	if ms.activeAlerts[1] == nil {
		t.Error("expected active alert for rule 1")
	}
}

func TestEvaluateOnce_NoFire(t *testing.T) {
	var webhookHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&webhookHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := newMockAlertStore()
	ms.rules = []store.AlertRule{
		{ID: 1, Metric: "cpu_pct", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: 10, Enabled: true},
	}
	ms.webhooks[10] = &store.Webhook{ID: 10, Type: "slack", URL: srv.URL}

	now := time.Now().Unix()
	mq := &mockMetricQuerier{
		points: []metrics.MetricPoint{
			{CPUPct: 50, Ts: now - 10},
			{CPUPct: 60, Ts: now - 20},
		},
	}
	al := &mockAppLookup{apps: make(map[int64]*store.App)}

	e := NewEvaluator(ms, al, mq, NewWebhookDispatcherAllowPrivate())
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce: %v", err)
	}

	if atomic.LoadInt32(&webhookHits) != 0 {
		t.Errorf("webhook hits = %d, want 0", webhookHits)
	}
	if len(ms.histories) != 0 {
		t.Errorf("histories = %d, want 0", len(ms.histories))
	}
}

func TestEvaluateOnce_ResolvesAlert(t *testing.T) {
	var webhookHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		atomic.AddInt32(&webhookHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ms := newMockAlertStore()
	ms.rules = []store.AlertRule{
		{ID: 1, Metric: "cpu_pct", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: 10, Enabled: true},
	}
	ms.webhooks[10] = &store.Webhook{ID: 10, Type: "slack", URL: srv.URL}

	// pre-existing active alert
	existing := &store.AlertHistory{ID: 99, RuleID: 1, FiredAt: time.Now().Add(-5 * time.Minute), Value: 90}
	ms.activeAlerts[1] = existing

	// metrics now below threshold
	now := time.Now().Unix()
	mq := &mockMetricQuerier{
		points: []metrics.MetricPoint{
			{CPUPct: 50, Ts: now - 10},
			{CPUPct: 60, Ts: now - 20},
		},
	}
	al := &mockAppLookup{apps: make(map[int64]*store.App)}

	e := NewEvaluator(ms, al, mq, NewWebhookDispatcherAllowPrivate())
	if err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatalf("EvaluateOnce: %v", err)
	}

	if atomic.LoadInt32(&webhookHits) != 1 {
		t.Errorf("webhook hits = %d, want 1 (resolved notification)", webhookHits)
	}
	if len(ms.resolvedIDs) != 1 || ms.resolvedIDs[0] != 99 {
		t.Errorf("resolvedIDs = %v, want [99]", ms.resolvedIDs)
	}
	if ms.activeAlerts[1] != nil {
		t.Error("expected no active alert after resolve")
	}
}
