package alerts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// nastyName breaks naive JSON interpolation: quotes, braces, backslashes,
// newlines, tabs, a control char, HTML-ish chars and a line separator.
const nastyName = "evil\"}, \"injected\": {\"x\": \"\\ \n\t\x01<b>&\u2028 app"

func captureSend(t *testing.T, wh store.Webhook, event AlertEvent) string {
	t.Helper()
	var captured, contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	wh.URL = srv.URL
	if err := NewWebhookDispatcherAllowPrivate().Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	return captured
}

// builtinPayloadKeys is the exact top-level key set of each builtin payload.
var builtinPayloadKeys = map[string][]string{
	"slack":    {"text"},
	"telegram": {"parse_mode", "text"},
	"discord":  {"content"},
	"custom":   {"app", "metric", "status", "threshold", "value"},
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func nastyEvent() AlertEvent {
	e := AlertEvent{
		AppName:   nastyName,
		AppSlug:   "app\"slug",
		Metric:    "cpu_pct",
		Value:     95.5,
		Threshold: 80,
		Operator:  ">\"",
		Status:    "firing",
		FiredAt:   time.Now(),
	}
	EnrichEvent(&e)
	return e
}

// TestBuiltinTemplatesEscapeValues: every builtin payload stays valid JSON,
// keeps its exact key set, and round-trips the raw app name.
func TestBuiltinTemplatesEscapeValues(t *testing.T) {
	for typ := range builtinTemplates {
		t.Run(typ, func(t *testing.T) {
			body := captureSend(t, store.Webhook{Type: typ}, nastyEvent())
			if !json.Valid([]byte(body)) {
				t.Fatalf("invalid JSON payload: %s", body)
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(body), &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if _, injected := m["injected"]; injected {
				t.Fatalf("app name injected a JSON key: %s", body)
			}
			wantKeys, ok := builtinPayloadKeys[typ]
			if !ok {
				t.Fatalf("no key set for builtin type %q; extend builtinPayloadKeys", typ)
			}
			if got := sortedKeys(m); !reflect.DeepEqual(got, wantKeys) {
				t.Errorf("payload keys = %v, want %v (%s)", got, wantKeys, body)
			}
			var text string
			switch typ {
			case "slack", "telegram":
				text, _ = m["text"].(string)
			case "discord":
				text, _ = m["content"].(string)
			case "custom":
				text, _ = m["app"].(string)
				if m["status"] != "firing" || m["metric"] != "cpu_pct" {
					t.Errorf("custom fields = %v", m)
				}
			default:
				t.Fatalf("no assertion for builtin type %q; extend this test", typ)
			}
			if !strings.Contains(text, nastyName) {
				t.Errorf("decoded payload does not contain the raw app name:\n got: %q\nwant substring: %q", text, nastyName)
			}
		})
	}
}

// TestBuiltinTemplatePayloads pins the full decoded payload of each builtin
// type (and the custom fallback for unknown types) for firing and resolved
// events.
func TestBuiltinTemplatePayloads(t *testing.T) {
	for _, status := range []string{"firing", "resolved"} {
		ev := makeEvent()
		ev.Status = status
		line := "[" + status + "] myapp - CPU > 80.0% (current: 95.5%)"
		custom := map[string]any{"app": "myapp", "metric": "cpu_pct", "value": 95.5, "threshold": 80.0, "status": status}
		want := map[string]map[string]any{
			"slack":    {"text": line},
			"telegram": {"text": "[" + status + "] myapp\nCPU > 80.0% (current: 95.5%)", "parse_mode": "HTML"},
			"discord":  {"content": line},
			"custom":   custom,
			"unknown":  custom, // unknown types fall back to the custom template
		}
		for typ := range builtinTemplates {
			if _, ok := want[typ]; !ok {
				t.Fatalf("no expected payload for builtin type %q; extend this test", typ)
			}
		}
		for typ, w := range want {
			t.Run(status+"/"+typ, func(t *testing.T) {
				body := captureSend(t, store.Webhook{Type: typ}, ev)
				var m map[string]any
				if err := json.Unmarshal([]byte(body), &m); err != nil {
					t.Fatalf("unmarshal: %v (%s)", err, body)
				}
				if !reflect.DeepEqual(m, w) {
					t.Errorf("payload = %v, want %v", m, w)
				}
			})
		}
	}
}

// TestOverrideTemplateReplacesBuiltin: a template override yields only its
// own keys, none of the builtin ones.
func TestOverrideTemplateReplacesBuiltin(t *testing.T) {
	for typ := range builtinTemplates {
		override := `{"custom":"tpl","metric":"{{.Metric}}","value":{{.Value}}}`
		body := captureSend(t, store.Webhook{Type: typ, TemplateOverride: override}, makeEvent())
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("%s: unmarshal: %v (%s)", typ, err, body)
		}
		want := map[string]any{"custom": "tpl", "metric": "cpu_pct", "value": 95.5}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("%s: payload = %v, want %v", typ, m, want)
		}
	}
}

// TestTelegramTemplateKeepsNewline: the literal \n escape in the telegram
// template still decodes to a newline.
func TestTelegramTemplateKeepsNewline(t *testing.T) {
	body := captureSend(t, store.Webhook{Type: "telegram"}, makeEvent())
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, body)
	}
	if text, _ := m["text"].(string); !strings.Contains(text, "myapp\nCPU") {
		t.Errorf("telegram text = %q", text)
	}
	if m["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %v", m["parse_mode"])
	}
}

// TestCustomOverrideTemplateEscapesValues: user JSON templates get the same
// escaping, so a crafted name cannot break their structure either.
func TestCustomOverrideTemplateEscapesValues(t *testing.T) {
	tmpl := `{"name":"{{.AppName}}","slug":"{{.AppSlug}}","op":"{{.Operator}}","value":{{printf "%.1f" .Value}},"when":"{{.FiredAt.Format "2006"}}"}`
	body := captureSend(t, store.Webhook{Type: "custom", TemplateOverride: tmpl}, nastyEvent())
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("invalid JSON from override template: %v\n%s", err, body)
	}
	if m["name"] != nastyName || m["slug"] != "app\"slug" || m["op"] != ">\"" || m["value"] != 95.5 {
		t.Errorf("decoded override payload = %v", m)
	}
}

// TestBackupAlertMessageEscaped: backup error messages (quotes, paths,
// newlines) produce valid payloads.
func TestBackupAlertMessageEscaped(t *testing.T) {
	ev := BackupAlertEvent{
		AppName:   "db \"prod\"",
		Message:   "pg_dump: error: \"relation\" missing\nexit status 1 (C:\\tmp)",
		EventType: "backup_failed",
		FiredAt:   time.Now(),
	}.ToAlertEvent()
	body := captureSend(t, store.Webhook{Type: "slack"}, ev)
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, body)
	}
	if text, _ := m["text"].(string); !strings.Contains(text, "exit status 1 (C:\\tmp)") {
		t.Errorf("text = %q", text)
	}
}

// TestJSONSafeEventCoversAllStringFields guards against new AlertEvent string
// fields being added without escaping.
func TestJSONSafeEventCoversAllStringFields(t *testing.T) {
	var e AlertEvent
	v := reflect.ValueOf(&e).Elem()
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String {
			v.Field(i).SetString(`"`)
		}
	}
	got := reflect.ValueOf(jsonSafeEvent(e))
	for i := 0; i < got.NumField(); i++ {
		if got.Field(i).Kind() != reflect.String {
			continue
		}
		if s := got.Field(i).String(); s != `\"` {
			t.Errorf("field %s not escaped: %q", got.Type().Field(i).Name, s)
		}
	}
}

func TestJSONEscape(t *testing.T) {
	cases := map[string]string{
		"plain":    "plain",
		`a"b`:      `a\"b`,
		`a\b`:      `a\\b`,
		"a\nb":     `a\nb`,
		"<b>&":     "<b>&",
		"\u2028":   `\u2028`,
		"\x01":     `\u0001`,
		"":         "",
		"unicode✓": "unicode✓",
	}
	for in, want := range cases {
		if got := jsonEscape(in); got != want {
			t.Errorf("jsonEscape(%q) = %q, want %q", in, got, want)
		}
	}
}
