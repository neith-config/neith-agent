package config

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/neith-config/neith-agent/internal/logger"
)


type lineScanner interface {
	Scan() bool
	Text() string
	Err() error
}

// DefaultSleep blocks for d or until ctx is cancelled, whichever comes first.
func DefaultSleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// HubSSEClient streams config updates from the Hub over Server-Sent Events.
// On any disconnect it reconnects with exponential backoff + jitter.
type HubSSEClient struct {
	hubURL       string
	token        string
	client       *http.Client
	logger       logger.Logger
	sleepFn      func(ctx context.Context, d time.Duration)
	jitterFn     func() float64
	initialDelay time.Duration
	maxDelay     time.Duration
	delayFactor  float64
	newScanner   func(io.Reader) lineScanner
}

// NewHubSSEClient creates a new HubSSEClient. All dependencies are injected by
// the caller so the constructor does not create its own transports or timers.
func NewHubSSEClient(
	hubURL, token string,
	client *http.Client,
	lg logger.Logger,
	sleepFn func(ctx context.Context, d time.Duration),
	jitterFn func() float64,
) *HubSSEClient {
	return &HubSSEClient{
		hubURL:       hubURL,
		token:        token,
		client:       client,
		logger:       lg,
		sleepFn:      sleepFn,
		jitterFn:     jitterFn,
		initialDelay: time.Second,
		maxDelay:     60 * time.Second,
		delayFactor:  2.0,
		newScanner:   func(r io.Reader) lineScanner { return bufio.NewScanner(r) },
	}
}

// Stream connects to the Hub SSE endpoint and calls onConfig for each received
// config event. It reconnects with exponential backoff+jitter on any disconnect.
// Stream returns only when ctx is cancelled.
func (c *HubSSEClient) Stream(ctx context.Context, onConfig func(map[string]string)) error {
	delay := c.initialDelay

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		receivedOne, err := c.connect(ctx, onConfig)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			c.logger.Error("SSE connection error", err)
		}

		if receivedOne {
			delay = c.initialDelay
		}

		jitteredDelay := time.Duration(float64(delay) * (0.8 + c.jitterFn()*0.4))
		c.logger.Debug(fmt.Sprintf("SSE reconnecting in %v", jitteredDelay))
		c.sleepFn(ctx, jitteredDelay)

		if !receivedOne {
			next := time.Duration(float64(delay) * c.delayFactor)
			if next > c.maxDelay {
				next = c.maxDelay
			}
			delay = next
		}
	}
}

// connect opens one SSE connection and drains events until the connection closes
// or ctx is cancelled. It returns whether at least one config event was received.
func (c *HubSSEClient) connect(ctx context.Context, onConfig func(map[string]string)) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.hubURL+"/sse/v1/stream", nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	receivedOne := false
	var dataLines []string

	scanner := c.newScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return receivedOne, nil
		}
		line := scanner.Text()

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			dataLines = append(dataLines, data)
		} else if line == "" && len(dataLines) > 0 {
			combined := strings.Join(dataLines, "")
			dataLines = dataLines[:0]

			var cfg map[string]string
			if err := json.Unmarshal([]byte(combined), &cfg); err != nil {
				c.logger.Error("SSE event parse error", err)
				continue
			}
			onConfig(cfg)
			receivedOne = true
		} else if line == "" {
			dataLines = dataLines[:0]
		}
	}

	if err := scanner.Err(); err != nil {
		return receivedOne, fmt.Errorf("read SSE stream: %w", err)
	}
	return receivedOne, nil
}
