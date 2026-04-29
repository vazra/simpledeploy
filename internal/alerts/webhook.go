package alerts

import (
	"bytes"
	"context"
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

// extraReservedRanges captures CIDRs not covered by net.IP.IsPrivate /
// IsLoopback / IsLinkLocal*: CGNAT, class-E, IETF assignments, and the
// AWS metadata equivalent on EC2 IMDSv2 alt path. Multicast and the
// unspecified address are checked separately via IsMulticast/IsUnspecified.
var extraReservedRanges = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8",
		"100.64.0.0/10",   // CGNAT
		"192.0.0.0/24",    // IETF
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // class E + broadcast
		"::/128",          // IPv6 unspecified
		"::1/128",         // IPv6 loopback
		"100::/64",        // IPv6 discard
		"fc00::/7",        // IPv6 unique-local
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// isReservedIP returns true if ip falls in any private, loopback, link-local,
// multicast, unspecified, or otherwise reserved range that should never be
// reachable from a webhook dispatcher.
func isReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range extraReservedRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

var builtinTemplates = map[string]string{
	"slack":    `{"text":"[{{.Status}}] {{.AppName}} - {{.MetricDisplay}} {{.Operator}} {{.ThresholdDisplay}} (current: {{.ValueDisplay}})"}`,
	"telegram": `{"text":"[{{.Status}}] {{.AppName}}\n{{.MetricDisplay}} {{.Operator}} {{.ThresholdDisplay}} (current: {{.ValueDisplay}})","parse_mode":"HTML"}`,
	"discord":  `{"content":"[{{.Status}}] {{.AppName}} - {{.MetricDisplay}} {{.Operator}} {{.ThresholdDisplay}} (current: {{.ValueDisplay}})"}`,
	"custom":   `{"app":"{{.AppName}}","metric":"{{.Metric}}","value":{{printf "%.2f" .Value}},"threshold":{{printf "%.2f" .Threshold}},"status":"{{.Status}}"}`,
}

type WebhookDispatcher struct {
	client       *http.Client
	allowPrivate bool // skip SSRF validation (for tests)
}

// safeDialer rejects dial attempts to reserved IPs at the transport layer,
// closing the DNS-rebinding window between validateWebhookURL's resolve and
// http.Client's resolve.
func safeDialer(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ipa := range ips {
			if !allowPrivate && isReservedIP(ipa.IP) {
				lastErr = fmt.Errorf("dial blocked: %s resolves to reserved IP %s", host, ipa.IP)
				continue
			}
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no valid address for %s", host)
		}
		return nil, lastErr
	}
}

func newDispatcherClient(allowPrivate bool) *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:           safeDialer(allowPrivate),
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			IdleConnTimeout:       30 * time.Second,
			MaxIdleConns:          10,
		},
	}
}

func NewWebhookDispatcher() *WebhookDispatcher {
	return &WebhookDispatcher{client: newDispatcherClient(false)}
}

// NewWebhookDispatcherAllowPrivate creates a dispatcher that skips SSRF checks (for testing).
func NewWebhookDispatcherAllowPrivate() *WebhookDispatcher {
	return &WebhookDispatcher{
		client:       newDispatcherClient(true),
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
		if isReservedIP(ip) {
			return fmt.Errorf("webhook url resolves to private/reserved IP %s", ip)
		}
	}
	return nil
}

// blockedWebhookHeaders are headers that cannot be overridden via webhook config.
var blockedWebhookHeaders = map[string]bool{
	"host":              true,
	"content-length":    true,
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
			if strings.ContainsAny(v, "\r\n") {
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
