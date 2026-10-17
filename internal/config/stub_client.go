package config

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const StubPollInterval = 3 * time.Second

// StubClient simulates a real config client backed by a master server.
// A background goroutine fires every StubPollInterval, increments the version,
// and replaces the in-memory snapshot — exactly as the real client will do on
// receiving an SSE update.
type StubClient struct {
	version atomic.Int64
	mu      sync.RWMutex
	config  map[string]string
}

func NewStubClient(ctx context.Context) *StubClient {
	c := &StubClient{}
	c.config = c.buildSnapshot()
	go c.run(ctx)
	return c
}

func (c *StubClient) run(ctx context.Context) {
	ticker := time.NewTicker(StubPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.version.Add(1)
			snapshot := c.buildSnapshot()
			c.mu.Lock()
			c.config = snapshot
			c.mu.Unlock()
		}
	}
}

func (c *StubClient) buildSnapshot() map[string]string {
	return map[string]string{
		"NEITH_STUB":    "true",
		"NEITH_VERSION": fmt.Sprintf("%d", c.version.Load()),
	}
}

func (c *StubClient) Fetch(_ context.Context) (map[string]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snapshot := make(map[string]string, len(c.config))
	for k, v := range c.config {
		snapshot[k] = v
	}
	return snapshot, nil
}
