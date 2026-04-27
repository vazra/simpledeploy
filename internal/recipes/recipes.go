// Package recipes is a small HTTP client for the SimpleDeploy community
// recipes catalog. The catalog is a static JSON index plus per-recipe
// compose.yml/README.md files served from GitHub Pages.
package recipes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout     = 10 * time.Second
	maxIndexBytes      = 2 * 1024 * 1024
	maxRecipeFileBytes = 256 * 1024
	userAgent          = "simpledeploy-recipes-client/1"
)

type Recipe struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Icon                   string   `json:"icon,omitempty"`
	Category               string   `json:"category"`
	Description            string   `json:"description"`
	Tags                   []string `json:"tags,omitempty"`
	Author                 string   `json:"author,omitempty"`
	Homepage               string   `json:"homepage,omitempty"`
	ComposeURL             string   `json:"compose_url"`
	ReadmeURL              string   `json:"readme_url"`
	ScreenshotURL          string   `json:"screenshot_url,omitempty"`
	SchemaVersion          int      `json:"schema_version,omitempty"`
	MinSimpleDeployVersion string   `json:"min_simpledeploy_version,omitempty"`
}

type Index struct {
	SchemaVersion int      `json:"schema_version"`
	GeneratedAt   string   `json:"generated_at"`
	Recipes       []Recipe `json:"recipes"`
}

type Client struct {
	indexURL string
	base     string
	http     *http.Client
}

func NewClient(indexURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = defaultTimeout
	}
	base := ""
	if u, err := url.Parse(indexURL); err == nil {
		base = u.Scheme + "://" + u.Host
	}
	return &Client{
		indexURL: indexURL,
		base:     base,
		http:     &http.Client{Timeout: timeout},
	}
}

func (c *Client) FetchIndex(ctx context.Context) (*Index, error) {
	body, err := c.get(ctx, c.indexURL, maxIndexBytes)
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	if idx.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported recipe index schema_version %d", idx.SchemaVersion)
	}
	return &idx, nil
}

// FetchText fetches a sub-resource (compose.yml, README.md). Refuses URLs
// outside the index host so a malicious index cannot make us fetch arbitrary
// internal endpoints.
func (c *Client) FetchText(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("invalid url")
	}
	if c.base != "" && !strings.HasPrefix(rawURL, c.base+"/") {
		return "", fmt.Errorf("url host not allowed")
	}
	body, err := c.get(ctx, rawURL, maxRecipeFileBytes)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) get(ctx context.Context, rawURL string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(body)) > max {
		return nil, fmt.Errorf("response exceeds %d bytes", max)
	}
	return body, nil
}
