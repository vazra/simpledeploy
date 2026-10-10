package alerts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	wh.URL = srv.URL
	if err := NewWebhookDispatcherAllowPrivate().Send(wh, event); err != nil {
		t.Fatalf("Send: %v", err)
	}
	return captured
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
// keeps its shape, and round-trips the raw app name.
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
