package config

import (
	"context"
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

// Fetch requests the legacy bare map response. The live agent uses
// FetchSnapshot so it can preserve the Hub revision across transports.
func (c *HubHTTPClient) Fetch(ctx context.Context) (map[string]string, error) {
	body, err := c.fetchBody(ctx)
	if err != nil {
		return nil, err
	}

	result, err := parseConfigMap(body)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return result, nil
}

// FetchSnapshot requests the authoritative revisioned snapshot used by the
// persistent agent lifecycle.
func (c *HubHTTPClient) FetchSnapshot(ctx context.Context) (Snapshot, error) {
	body, err := c.fetchBody(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	return parseSnapshotEnvelope(body)
}

func (c *HubHTTPClient) fetchBody(ctx context.Context) ([]byte, error) {
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

	body, err := readAll(io.LimitReader(resp.Body, MaxSnapshotPayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if len(body) > MaxSnapshotPayloadBytes {
		return nil, fmt.Errorf("response payload exceeds %d bytes", MaxSnapshotPayloadBytes)
	}
	return body, nil
}
