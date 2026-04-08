package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type AppInfo struct {
	ID     int64  `json:"ID"`
	Name   string `json:"Name"`
	Slug   string `json:"Slug"`
	Status string `json:"Status"`
	Domain string `json:"Domain"`
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ListApps() ([]AppInfo, error) {
	var apps []AppInfo
	if err := c.get("/api/apps", &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

func (c *Client) GetApp(slug string) (*AppInfo, error) {
	var app AppInfo
	if err := c.get("/api/apps/"+slug, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

func (c *Client) DeployApp(name string, composeData []byte) error {
	body := map[string]string{
		"name":    name,
		"compose": base64.StdEncoding.EncodeToString(composeData),
	}
	return c.post("/api/apps/deploy", body)
}

func (c *Client) RemoveApp(slug string) error {
	return c.delete("/api/apps/" + slug)
}

func (c *Client) GetAppCompose(slug string) ([]byte, error) {
	resp, err := c.do("GET", "/api/apps/"+slug+"/compose", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) TriggerBackup(slug string) error {
	return c.post("/api/apps/"+slug+"/backups/run", nil)
}

func (c *Client) ListBackupRuns(slug string) ([]json.RawMessage, error) {
	var runs []json.RawMessage
	if err := c.get("/api/apps/"+slug+"/backups/runs", &runs); err != nil {
		return nil, err
	}
	return runs, nil
}

func (c *Client) Restore(runID int64) error {
	return c.post(fmt.Sprintf("/api/backups/restore/%d", runID), nil)
}

func (c *Client) do(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(b))
	}
	return resp, nil
}

func (c *Client) get(path string, result interface{}) error {
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) post(path string, body interface{}) error {
	resp, err := c.do("POST", path, body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) delete(path string) error {
	resp, err := c.do("DELETE", path, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
