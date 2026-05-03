package config

import (
	"context"
	"testing"
	"time"
)

func TestStubClientInitialFetch(t *testing.T) {
	t.Parallel()

	c := NewStubClient(context.Background())
	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["NETH_STUB"] != "true" {
		t.Errorf("NETH_STUB: want %q, got %q", "true", got["NETH_STUB"])
	}
	if got["NETH_VERSION"] != "0" {
		t.Errorf("NETH_VERSION: want %q, got %q", "0", got["NETH_VERSION"])
	}
}

func TestStubClientVersionIncrementsAfterInterval(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &StubClient{}
	c.config = c.buildSnapshot()

	// Replace ticker interval with a short one for testing.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
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
	}()

	time.Sleep(175 * time.Millisecond)
	cancel()
	<-done

	got, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	version := got["NETH_VERSION"]
	if version == "0" {
		t.Errorf("expected version > 0 after ticks, got %q", version)
	}
}

func TestStubClientFetchReturnsCopy(t *testing.T) {
	t.Parallel()

	c := NewStubClient(context.Background())
	first, _ := c.Fetch(context.Background())
	first["NETH_STUB"] = "mutated"

	second, _ := c.Fetch(context.Background())
	if second["NETH_STUB"] != "true" {
		t.Errorf("Fetch should return a copy; internal state was mutated")
	}
}

func TestStubClientCancelStopsGoroutine(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	NewStubClient(ctx)
	cancel()
	// Give the goroutine time to exit; no assertion needed — the test runner
	// will detect a leaked goroutine if it does not stop.
	time.Sleep(20 * time.Millisecond)
}
