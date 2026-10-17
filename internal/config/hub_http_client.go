package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var readAll = io.ReadAll

// HubHTTPClient fetches a config snapshot from the Hub in a single HTTP request.
// It implements runner.EnvFetcher and is used for jitter-polling fallback.
type HubHTTPClient struct {
	hubURL string
	token  string
	client *http.Client
}

// NewHubHTTPClient creates a new HubHTTPClient. client is injected by the caller
// so the constructor does not create its own HTTP transport.
func NewHubHTTPClient(hubURL, token string, client *http.Client) *HubHTTPClient {
	return &HubHTTPClient{hubURL: hubURL, token: token, client: client}
}

// Fetch requests the current config snapshot from the Hub and returns it as a
// key-value map. It satisfies runner.EnvFetcher.
func (c *HubHTTPClient) Fetch(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.hubURL+"/api/v1/snapshot", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := readAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}
