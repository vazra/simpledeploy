package alerts

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

func makeEvent() AlertEvent {
	e := AlertEvent{
		AppName:   "myapp",
		AppSlug:   "myapp",
		Metric:    "cpu_pct",
		Value:     95.5,
		Threshold: 80.0,
		Operator:  ">",
		Status:    "firing",
		FiredAt:   time.Now(),
	}
	EnrichEvent(&e)
	return e
}

func renderTemplate(t *testing.T, tmplStr string, event AlertEvent) string {
	t.Helper()
	d := NewWebhookDispatcherAllowPrivate()
	var srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Body", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := store.Webhook{
		Type:             "custom",
		URL:              srv.URL,
		TemplateOverride: tmplStr,
	}

	var captured string
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.WriteHeader(http.StatusOK)
	})

	if err := d.Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}
	return captured
}

func TestRenderSlackTemplate(t *testing.T) {
	event := makeEvent()
	d := NewWebhookDispatcherAllowPrivate()

	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := store.Webhook{Type: "slack", URL: srv.URL}
	if err := d.Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !strings.Contains(captured, `"text"`) {
		t.Errorf("expected 'text' field, got: %s", captured)
	}
	if !strings.Contains(captured, "[firing]") {
		t.Errorf("expected '[firing]' in body, got: %s", captured)
	}
	if !strings.Contains(captured, "myapp") {
		t.Errorf("expected app name in body, got: %s", captured)
	}
	if !strings.Contains(captured, "95.5") {
		t.Errorf("expected value 95.5 in body, got: %s", captured)
	}
}

func TestRenderCustomTemplate(t *testing.T) {
	event := makeEvent()
	d := NewWebhookDispatcherAllowPrivate()

	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := store.Webhook{Type: "custom", URL: srv.URL}
	if err := d.Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !strings.Contains(captured, `"app"`) {
		t.Errorf("expected 'app' field, got: %s", captured)
	}
	if !strings.Contains(captured, `"metric"`) {
		t.Errorf("expected 'metric' field, got: %s", captured)
	}
	if !strings.Contains(captured, "95.50") {
		t.Errorf("expected value 95.50 in body, got: %s", captured)
	}
	if !strings.Contains(captured, `"firing"`) {
		t.Errorf("expected status 'firing' in body, got: %s", captured)
	}
}

func TestRenderWithOverride(t *testing.T) {
	event := makeEvent()
	override := `{"custom_field":"{{.AppName}}","status":"{{.Status}}"}`
	body := renderTemplate(t, override, event)

	if !strings.Contains(body, `"custom_field"`) {
		t.Errorf("expected 'custom_field' in body, got: %s", body)
	}
	if !strings.Contains(body, "myapp") {
		t.Errorf("expected 'myapp' in body, got: %s", body)
	}
	if strings.Contains(body, `"text"`) {
		t.Errorf("override should not produce 'text' field, got: %s", body)
	}
}

func TestWebhookSend(t *testing.T) {
	event := makeEvent()
	d := NewWebhookDispatcherAllowPrivate()

	var gotMethod, gotContentType, gotBody, gotCustomHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotCustomHeader = r.Header.Get("X-Custom")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := store.Webhook{
		Type:        "slack",
		URL:         srv.URL,
		HeadersJSON: `{"X-Custom":"testvalue"}`,
	}
	if err := d.Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotCustomHeader != "testvalue" {
		t.Errorf("X-Custom = %q, want testvalue", gotCustomHeader)
	}
	if !strings.Contains(gotBody, "myapp") {
		t.Errorf("body missing app name: %s", gotBody)
	}
}

func TestWebhookSendError(t *testing.T) {
	event := makeEvent()
	d := NewWebhookDispatcherAllowPrivate()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	wh := store.Webhook{Type: "slack", URL: srv.URL}
	err := d.Send(wh, event)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestIsReservedIP(t *testing.T) {
	cases := []struct {
		ip       string
		reserved bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true}, // CGNAT
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"203.0.113.5", true}, // TEST-NET-3
		{"224.0.0.1", true},   // multicast
		{"::1", true},
		{"fc00::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if got := isReservedIP(ip); got != c.reserved {
			t.Errorf("isReservedIP(%s) = %v, want %v", c.ip, got, c.reserved)
		}
	}
}
