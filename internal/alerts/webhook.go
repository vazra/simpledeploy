package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	client       *http.Client
	allowPrivate bool // skip SSRF validation (for tests)
}

func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{client: &http.Client{Timeout: 10 * time.Second}}
}

// NewWebhookDispatcherAllowPrivate creates a dispatcher that skips SSRF checks (for testing).
func NewWebhookDispatcherAllowPrivate() *WebhookDispatcher {
	return &WebhookDispatcher{
		client:       &http.Client{Timeout: 10 * time.Second},
		allowPrivate: true,
	}
}

// validateWebhookURL rejects URLs that target private/reserved IPs or non-HTTP schemes.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme %q not allowed, must be http or https", u.Scheme)
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("dns lookup failed for %q: %w", host, err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("webhook url resolves to private/reserved IP %s", ip)
		}
		// Block cloud metadata IPs (169.254.169.254)
		if ip.Equal(net.ParseIP("169.254.169.254")) {
			return fmt.Errorf("webhook url resolves to cloud metadata IP")
		}
	}
	return nil
}

// blockedWebhookHeaders are headers that cannot be overridden via webhook config.
var blockedWebhookHeaders = map[string]bool{
	"host":           true,
	"content-length": true,
	"transfer-encoding": true,
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

	if !d.allowPrivate {
		if err := validateWebhookURL(webhook.URL); err != nil {
			return fmt.Errorf("webhook url rejected: %w", err)
		}
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
			if blockedWebhookHeaders[strings.ToLower(k)] {
				continue
			}
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
