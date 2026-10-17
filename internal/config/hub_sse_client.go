package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/neith-config/neith-agent/internal/logger"
)

const (
	DefaultSSEIdleTimeout   = 2 * time.Minute
	MaxSnapshotPayloadBytes = 1 << 20
	maxSSELineBytes         = MaxSnapshotPayloadBytes + len("data: ") + 1
)

var errSSEIdleTimeout = errors.New("SSE stream idle timeout")

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
type sseIdleTimer interface {
	Reset(time.Duration) bool
	Stop() bool
}

type realSSEIdleTimer struct {
	timer *time.Timer
}

func (t *realSSEIdleTimer) Reset(d time.Duration) bool { return t.timer.Reset(d) }
func (t *realSSEIdleTimer) Stop() bool                 { return t.timer.Stop() }

func newSSEIdleTimer(d time.Duration, onIdle func()) sseIdleTimer {
	return &realSSEIdleTimer{timer: time.AfterFunc(d, onIdle)}
}

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
	idleTimeout  time.Duration
	newIdleTimer func(time.Duration, func()) sseIdleTimer
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
		idleTimeout:  DefaultSSEIdleTimeout,
		newIdleTimer: newSSEIdleTimer,
		newScanner: func(r io.Reader) lineScanner {
			scanner := bufio.NewScanner(r)
			scanner.Buffer(make([]byte, 64*1024), maxSSELineBytes)
			return scanner
		},
	}
}

// Stream connects to the Hub SSE endpoint and calls onConfig for each received
// config event. It reconnects with exponential backoff+jitter on any disconnect.
// Stream returns only when ctx is cancelled.
func (c *HubSSEClient) Stream(ctx context.Context, onConfig func(map[string]string)) error {
	return c.stream(ctx, func(data string) bool {
		cfg, err := parseConfigMap([]byte(data))
		if err != nil {
			c.logger.Error("SSE event parse error", err)
			return false
		}
		onConfig(cfg)
		return true
	})
}

// StreamSnapshots streams revisioned Hub snapshots while preserving the
// authoritative revision in each callback. The legacy Stream method remains
// available for disconnected map-based runner components.
func (c *HubSSEClient) StreamSnapshots(ctx context.Context, onSnapshot func(Snapshot)) error {
	return c.stream(ctx, func(data string) bool {
		snapshot, err := parseSnapshotEnvelope([]byte(data))
		if err != nil {
			c.logger.Error("SSE snapshot parse error", err)
			return false
		}
		onSnapshot(snapshot)
		return true
	})
}

func (c *HubSSEClient) stream(ctx context.Context, onEvent func(string) bool) error {
	delay := c.initialDelay

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		receivedOne, err := c.connectEvents(ctx, onEvent)
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

// connect opens one SSE connection and drains legacy bare-map events until the
// connection closes or ctx is cancelled.
func (c *HubSSEClient) connect(ctx context.Context, onConfig func(map[string]string)) (bool, error) {
	return c.connectEvents(ctx, func(data string) bool {
		cfg, err := parseConfigMap([]byte(data))
		if err != nil {
			c.logger.Error("SSE event parse error", err)
			return false
		}
		onConfig(cfg)
		return true
	})
}

func sseDataValue(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	value := strings.TrimPrefix(line, "data:")
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	return value, true
}

// connectEvents opens one SSE connection and passes each complete event to
// onEvent. The callback returns true only when the event was accepted. Any SSE
// line, including a comment heartbeat, resets the idle timer; the stream has no
// total lifetime limit and ends on cancellation, disconnect, or idle timeout.
func (c *HubSSEClient) connectEvents(ctx context.Context, onEvent func(string) bool) (bool, error) {
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
	dataSize := 0
	var idle atomic.Bool
	var idleTimer sseIdleTimer
	if c.idleTimeout > 0 && c.newIdleTimer != nil {
		idleTimer = c.newIdleTimer(c.idleTimeout, func() {
			idle.Store(true)
			_ = resp.Body.Close()
		})
		defer idleTimer.Stop()
	}

	scanner := c.newScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return receivedOne, nil
		}
		if idleTimer != nil {
			idleTimer.Reset(c.idleTimeout)
		}
		line := scanner.Text()

		if data, ok := sseDataValue(line); ok {
			lineSize := len(data)
			if len(dataLines) > 0 {
				lineSize++ // SSE inserts a newline between data fields.
			}
			if dataSize+lineSize > MaxSnapshotPayloadBytes {
				return receivedOne, fmt.Errorf("SSE event payload exceeds %d bytes", MaxSnapshotPayloadBytes)
			}
			dataLines = append(dataLines, data)
			dataSize += lineSize
		} else if line == "" && len(dataLines) > 0 {
			// SSE joins consecutive data fields with a newline before dispatch.
			combined := strings.Join(dataLines, "\n")
			dataLines = dataLines[:0]
			dataSize = 0

			if onEvent(combined) {
				receivedOne = true
			}
		} else if line == "" {
			dataLines = dataLines[:0]
			dataSize = 0
		}
	}

	if idle.Load() {
		return receivedOne, fmt.Errorf("%w after %v", errSSEIdleTimeout, c.idleTimeout)
	}
	if err := scanner.Err(); err != nil {
		return receivedOne, fmt.Errorf("read SSE stream: %w", err)
	}
	return receivedOne, nil
}
