package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type noOpLogger struct{}

func (noOpLogger) Debug(string)            {}
func (noOpLogger) Info(string)             {}
func (noOpLogger) Warning(string)          {}
func (noOpLogger) Error(string, error)     {}

func noOpSleep(_ context.Context, _ time.Duration) {}

func fixedJitter() float64 { return 0.5 } // delay * (0.8 + 0.5*0.4) = delay * 1.0

func TestNewHubSSEClient(t *testing.T) {
	t.Parallel()
	c := NewHubSSEClient("http://hub", "tok", &http.Client{}, noOpLogger{}, noOpSleep, fixedJitter)
	if c.hubURL != "http://hub" || c.token != "tok" {
		t.Fatal("fields not stored")
	}
}

func TestDefaultSleep(t *testing.T) {
	t.Parallel()

	t.Run("returns after duration", func(t *testing.T) {
		ctx := context.Background()
		start := time.Now()
		DefaultSleep(ctx, 10*time.Millisecond)
		if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
			t.Fatalf("expected sleep >= 10ms, got %v", elapsed)
		}
	})

	t.Run("returns early on context cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(5 * time.Millisecond)
			cancel()
		}()
		start := time.Now()
		DefaultSleep(ctx, 10*time.Second)
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Fatalf("expected early return, took %v", elapsed)
		}
	})
}

func TestHubSSEClientStreamSingleEvent(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"KEY\":\"VAL\"}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	var received map[string]string
	c.Stream(ctx, func(cfg map[string]string) { //nolint
		received = cfg
		cancel()
	})

	if received["KEY"] != "VAL" {
		t.Fatalf("expected KEY=VAL, got %v", received)
	}
}

func TestHubSSEClientStreamSnapshots(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"revision\":\"018f0000-0000-7000-8000-000000000001\",\"config\":{\"KEY\":\"VALUE\"}}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	var received Snapshot
	c.StreamSnapshots(ctx, func(snapshot Snapshot) {
		received = snapshot
		cancel()
	})

	if received.Revision.String() != "018f0000-0000-7000-8000-000000000001" || received.Config["KEY"] != "VALUE" {
		t.Fatalf("unexpected snapshot: %#v", received)
	}
}

func TestHubSSEClientStreamMultipleEvents(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"N\":\"1\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"N\":\"2\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"N\":\"3\"}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	var mu sync.Mutex
	var received []string
	c.Stream(ctx, func(cfg map[string]string) { //nolint
		mu.Lock()
		received = append(received, cfg["N"])
		mu.Unlock()
		if len(received) == 3 {
			cancel()
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}
	if received[0] != "1" || received[1] != "2" || received[2] != "3" {
		t.Fatalf("unexpected order: %v", received)
	}
}

func TestHubSSEClientStreamReconnectsAfterDisconnect(t *testing.T) {
	t.Parallel()
	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"N\":\"%d\"}\n\n", n)
		flusher.Flush()
		if n == 1 {
			return // close first connection immediately
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	var mu sync.Mutex
	var received []string
	c.Stream(ctx, func(cfg map[string]string) { //nolint
		mu.Lock()
		received = append(received, cfg["N"])
		mu.Unlock()
		if cfg["N"] == "2" {
			cancel()
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("expected reconnect with second event, got %v", received)
	}
}

func TestHubSSEClientStreamMalformedJSONSkipped(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: not-json\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"OK\":\"yes\"}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	var received map[string]string
	c.Stream(ctx, func(cfg map[string]string) { //nolint
		received = cfg
		cancel()
	})

	if received["OK"] != "yes" {
		t.Fatalf("expected OK=yes after skipping malformed event, got %v", received)
	}
}

func TestHubSSEClientStreamContextCancelled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)

	err := c.Stream(ctx, func(map[string]string) {})
	if err == nil {
		t.Fatal("expected error on pre-cancelled context")
	}
}

func TestHubSSEClientStreamBackoffOnFailure(t *testing.T) {
	t.Parallel()

	// server always closes immediately with no events
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer srv.Close()

	var mu sync.Mutex
	var sleepDurations []time.Duration
	recordSleep := func(_ context.Context, d time.Duration) {
		mu.Lock()
		sleepDurations = append(sleepDurations, d)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, recordSleep, fixedJitter)
	c.initialDelay = 10 * time.Millisecond
	c.maxDelay = 40 * time.Millisecond
	wantInitial := c.initialDelay

	c.Stream(ctx, func(map[string]string) {}) //nolint

	mu.Lock()
	defer mu.Unlock()
	if len(sleepDurations) < 2 {
		t.Fatalf("expected at least 2 sleep calls for backoff, got %d", len(sleepDurations))
	}
	if sleepDurations[0] != wantInitial {
		t.Fatalf("expected first sleep %v, got %v", wantInitial, sleepDurations[0])
	}
	if sleepDurations[1] != 2*wantInitial {
		t.Fatalf("expected second sleep %v, got %v", 2*wantInitial, sleepDurations[1])
	}
}

func TestHubSSEClientStreamBackoffResetAfterSuccess(t *testing.T) {
	t.Parallel()

	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		if n == 1 {
			// successful connection
			fmt.Fprintf(w, "data: {\"N\":\"1\"}\n\n")
			flusher.Flush()
			return
		}
		// subsequent: close with no events (simulate failure)
		<-r.Context().Done()
	}))
	defer srv.Close()

	var mu sync.Mutex
	var sleepDurations []time.Duration
	recordSleep := func(_ context.Context, d time.Duration) {
		mu.Lock()
		sleepDurations = append(sleepDurations, d)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, recordSleep, fixedJitter)
	c.initialDelay = 10 * time.Millisecond
	c.maxDelay = 100 * time.Millisecond
	wantInitial := c.initialDelay

	c.Stream(ctx, func(map[string]string) {}) //nolint

	mu.Lock()
	defer mu.Unlock()
	if len(sleepDurations) < 1 {
		t.Fatalf("expected at least 1 sleep call, got %d", len(sleepDurations))
	}
	// After a successful first connection, delay resets to initial
	if sleepDurations[0] != wantInitial {
		t.Fatalf("expected delay reset to initial %v after success, got %v", wantInitial, sleepDurations[0])
	}
}

func TestHubSSEClientStreamNonOKStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	var sleepCalled atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{},
		func(_ context.Context, _ time.Duration) { sleepCalled.Store(true); cancel() },
		fixedJitter,
	)
	c.Stream(ctx, func(map[string]string) {}) //nolint

	if !sleepCalled.Load() {
		t.Fatal("expected sleep to be called after non-OK status")
	}
}

func TestHubSSEClientStreamDelayCapAtMax(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer srv.Close()

	var mu sync.Mutex
	var sleepDurations []time.Duration
	recordSleep := func(_ context.Context, d time.Duration) {
		mu.Lock()
		sleepDurations = append(sleepDurations, d)
		mu.Unlock()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, recordSleep, fixedJitter)
	c.initialDelay = 10 * time.Millisecond
	c.maxDelay = 20 * time.Millisecond
	wantMax := c.maxDelay

	c.Stream(ctx, func(map[string]string) {}) //nolint

	mu.Lock()
	defer mu.Unlock()
	// After enough failures delay should cap at maxDelay
	for i, d := range sleepDurations {
		if d > wantMax {
			t.Fatalf("sleep[%d]=%v exceeds maxDelay=%v", i, d, wantMax)
		}
	}
	capped := false
	for _, d := range sleepDurations {
		if d == wantMax {
			capped = true
		}
	}
	if !capped {
		t.Fatalf("expected at least one sleep at maxDelay=%v, got %v", wantMax, sleepDurations)
	}
}

func TestHubSSEClientStreamBadURL(t *testing.T) {
	t.Parallel()
	// Control character makes http.NewRequestWithContext fail immediately.
	c := NewHubSSEClient("http://\x00invalid", "tok", &http.Client{}, noOpLogger{}, noOpSleep, fixedJitter)

	ctx, cancel := context.WithCancel(context.Background())
	c.sleepFn = func(_ context.Context, _ time.Duration) { cancel() }

	c.Stream(ctx, func(map[string]string) {}) //nolint
}

// fakeScanner is an injected lineScanner for error-path testing.
type fakeScanner struct {
	lines []string
	pos   int
	err   error
}

func (f *fakeScanner) Scan() bool  { return f.pos < len(f.lines) }
func (f *fakeScanner) Text() string { line := f.lines[f.pos]; f.pos++; return line }
func (f *fakeScanner) Err() error  { return f.err }

func TestHubSSEClientStreamContextCancelledDuringScan(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{}, noOpSleep, fixedJitter)
	// Scanner has a leading empty line (triggers the bare-separator path), then two
	// complete events. onConfig cancels ctx on the first dispatch; the second data
	// line triggers the ctx.Err() != nil check inside the scan loop.
	c.newScanner = func(_ io.Reader) lineScanner {
		return &fakeScanner{lines: []string{``, `data: {"K":"V"}`, ``, `data: {"K":"V2"}`, ``}}
	}
	c.Stream(ctx, func(map[string]string) { cancel() }) //nolint
}

func TestHubSSEClientStreamScannerError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scanErr := errors.New("injected scan error")
	c := NewHubSSEClient(srv.URL, "tok", srv.Client(), noOpLogger{},
		func(_ context.Context, _ time.Duration) { cancel() },
		fixedJitter,
	)
	c.newScanner = func(_ io.Reader) lineScanner {
		return &fakeScanner{err: scanErr}
	}
	c.Stream(ctx, func(map[string]string) {}) //nolint
}
