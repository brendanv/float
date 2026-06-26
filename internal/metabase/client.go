package metabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal Metabase REST API client scoped to what the Custom
// Dashboards handoff needs: ensuring a SQLite database connection exists and
// triggering a schema re-sync after the export file changes. Authentication
// uses a Metabase API key sent in the x-api-key header.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient builds a client for the given Metabase base URL (e.g.
// http://metabase:3000) and API key.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type database struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Engine string `json:"engine"`
}

// EnsureDatabase returns the id of the Metabase database connection named name,
// creating a SQLite connection pointed at dbPath if none exists. dbPath is the
// absolute path to the SQLite file as seen from inside the Metabase container.
func (c *Client) EnsureDatabase(ctx context.Context, name, dbPath string) (int, error) {
	id, err := c.findDatabase(ctx, name)
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	body := map[string]any{
		"name":   name,
		"engine": "sqlite",
		"details": map[string]any{
			"db": dbPath,
		},
	}
	var created database
	if err := c.do(ctx, http.MethodPost, "/api/database", body, &created); err != nil {
		return 0, fmt.Errorf("metabase: create database: %w", err)
	}
	if created.ID == 0 {
		return 0, fmt.Errorf("metabase: create database returned no id")
	}
	return created.ID, nil
}

// findDatabase returns the id of the database named name, or 0 if absent.
func (c *Client) findDatabase(ctx context.Context, name string) (int, error) {
	raw, err := c.doRaw(ctx, http.MethodGet, "/api/database", nil)
	if err != nil {
		return 0, fmt.Errorf("metabase: list databases: %w", err)
	}
	// Modern Metabase wraps the list as {"data": [...], "total": n}; older
	// versions return a bare array. Accept both.
	var wrapped struct {
		Data []database `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Data != nil {
		return matchDatabase(wrapped.Data, name), nil
	}
	var list []database
	if err := json.Unmarshal(raw, &list); err != nil {
		return 0, fmt.Errorf("metabase: parse database list: %w", err)
	}
	return matchDatabase(list, name), nil
}

func matchDatabase(dbs []database, name string) int {
	for _, db := range dbs {
		if db.Name == name {
			return db.ID
		}
	}
	return 0
}

// SyncSchema triggers an asynchronous schema re-sync for the given database so
// Metabase picks up data written by a fresh export.
func (c *Client) SyncSchema(ctx context.Context, id int) error {
	path := fmt.Sprintf("/api/database/%d/sync_schema", id)
	if err := c.do(ctx, http.MethodPost, path, nil, nil); err != nil {
		return fmt.Errorf("metabase: sync schema: %w", err)
	}
	return nil
}

// do performs a JSON request and optionally decodes the response into out.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	raw, err := c.doRaw(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// doRaw performs a request and returns the raw response body.
func (c *Client) doRaw(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}
