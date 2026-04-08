package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

var builtinTemplates = map[string]string{
	"slack":    `{"text":"[{{.Status}}] {{.AppName}} - {{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf "%.1f" .Value}})"}`,
	"telegram": `{"text":"[{{.Status}}] {{.AppName}}\n{{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf "%.1f" .Value}})","parse_mode":"HTML"}`,
	"discord":  `{"content":"[{{.Status}}] {{.AppName}} - {{.Metric}} {{.Operator}} {{.Threshold}} (current: {{printf "%.1f" .Value}})"}`,
	"custom":   `{"app":"{{.AppName}}","metric":"{{.Metric}}","value":{{printf "%.2f" .Value}},"threshold":{{printf "%.2f" .Threshold}},"status":"{{.Status}}"}`,
}

type WebhookDispatcher struct {
	client *http.Client
}

func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{client: &http.Client{Timeout: 10 * time.Second}}
}

func (d *WebhookDispatcher) Send(webhook store.Webhook, event AlertEvent) error {
	tmplStr := webhook.TemplateOverride
	if tmplStr == "" {
		var ok bool
		tmplStr, ok = builtinTemplates[webhook.Type]
		if !ok {
			tmplStr = builtinTemplates["custom"]
		}
	}

	tmpl, err := template.New("webhook").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, event); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhook.URL, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if webhook.HeadersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(webhook.HeadersJSON), &headers); err != nil {
			return fmt.Errorf("parse headers: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
