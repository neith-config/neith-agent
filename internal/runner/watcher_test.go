package runner

import (
	"context"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// watcherFetcher is a test fetcher whose response can be changed between calls.
type watcherFetcher struct {
	mu     atomic.Value // stores map[string]string
	err    error
	calls  atomic.Int32
}

func newWatcherFetcher(cfg map[string]string) *watcherFetcher {
	f := &watcherFetcher{}
	f.mu.Store(cfg)
	return f
}

func (f *watcherFetcher) setConfig(cfg map[string]string) { f.mu.Store(cfg) }

func (f *watcherFetcher) Fetch(_ context.Context) (map[string]string, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return f.mu.Load().(map[string]string), nil
}

func TestNewConfigWatcher(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 100*time.Millisecond)
	if cw == nil {
		t.Fatal("expected non-nil ConfigWatcher")
	}
}

func TestConfigWatcherAddCallback(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 100*time.Millisecond)

	called := make(chan map[string]string, 1)
	cw.AddCallback(func(cfg map[string]string) { called <- cfg })

	cw.mu.RLock()
	n := len(cw.callbacks)
	cw.mu.RUnlock()

	if n != 1 {
		t.Fatalf("expected 1 callback, got %d", n)
	}
}

func TestConfigWatcherStartStop(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 50*time.Millisecond)

	cw.Start(context.Background())
	cw.Start(context.Background()) // second Start is a no-op

	time.Sleep(20 * time.Millisecond)
	cw.Stop()
	cw.Stop() // second Stop is a no-op
}

func TestConfigWatcherDetectsChange(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 30*time.Millisecond)

	var changeCount atomic.Int32
	cw.AddCallback(func(_ map[string]string) { changeCount.Add(1) })

	cw.Start(context.Background())
	defer cw.Stop()

	// Let the initial fetch run and stabilise.
	time.Sleep(20 * time.Millisecond)
	initial := changeCount.Load()

	// Mutate config so the hash changes.
	fetcher.setConfig(map[string]string{"A": "2"})

	// Wait for at least one tick to detect the change.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if changeCount.Load() > initial {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("callback not called after config change within 500ms")
}

func TestConfigWatcherFetchError(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{})
	lg := &testLogger{}
	cw := NewConfigWatcher(fetcher, lg, 20*time.Millisecond)

	// Let first checkConfig run (no error yet).
	cw.Start(context.Background())
	time.Sleep(10 * time.Millisecond)

	// Inject error.
	fetcher.err = errTest
	time.Sleep(50 * time.Millisecond)
	cw.Stop()

	if len(lg.errors) == 0 {
		t.Fatal("expected errors logged for fetch failure")
	}
}

var errTest = &testErr{msg: "test error"}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

func TestConfigWatcherHashConfig(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)

	// Empty map always hashes to the same value.
	h1 := cw.hashConfig(map[string]string{})
	h2 := cw.hashConfig(map[string]string{})
	if h1 != h2 {
		t.Fatal("empty config should produce consistent hash")
	}

	// Non-empty map produces some hash (just exercise the loop).
	_ = cw.hashConfig(map[string]string{"A": "1"})
	_ = cw.hashConfig(map[string]string{"A": "1", "B": "2"})
}

func TestNewProcessRestarter(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", []string{"hi"}, RunConfig{})
	if pr == nil {
		t.Fatal("expected non-nil ProcessRestarter")
	}
}

func TestProcessRestarterStartStop(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses unix-style echo command")
	}

	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	oldStart := startCmd
	oldWait := waitCmd
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
		startCmd = oldStart
		waitCmd = oldWait
	}()

	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, args...)
	}
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(c *exec.Cmd) error { return c.Start() }
	waitCmd = func(c *exec.Cmd) error { return c.Wait() }

	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 100*time.Millisecond)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", []string{"hi"}, RunConfig{
		Command: "echo",
		Args:    []string{"hi"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := pr.Start(ctx, fetcher); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pr.Stop()
}

func TestProcessRestarterStartAlreadyRunning(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses unix-style echo command")
	}

	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	oldStart := startCmd
	oldWait := waitCmd
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
		startCmd = oldStart
		waitCmd = oldWait
	}()

	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, args...)
	}
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(c *exec.Cmd) error { return c.Start() }
	waitCmd = func(c *exec.Cmd) error { return c.Wait() }

	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", []string{"hi"}, RunConfig{
		Command: "echo",
		Args:    []string{"hi"},
	})

	ctx := context.Background()
	_ = pr.Start(ctx, fetcher)
	// Second Start should return nil (already running).
	if err := pr.Start(ctx, fetcher); err != nil {
		t.Fatalf("second Start should be no-op, got: %v", err)
	}
	pr.Stop()
}

func TestProcessRestarterStartError(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "__nonexistent__", nil, RunConfig{
		Command: "__nonexistent__",
	})

	err := pr.Start(context.Background(), fetcher)
	// Run() returns non-zero exit code for missing command → ProcessError
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestProcessErrorMessage(t *testing.T) {
	t.Parallel()
	pe := &ProcessError{ExitCode: 127, Cmd: "missing"}
	msg := pe.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestProcessRestarterOnConfigChange(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses unix-style echo command")
	}

	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	oldStart := startCmd
	oldWait := waitCmd
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
		startCmd = oldStart
		waitCmd = oldWait
	}()

	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, args...)
	}
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(c *exec.Cmd) error { return c.Start() }
	waitCmd = func(c *exec.Cmd) error { return c.Wait() }

	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, 100*time.Millisecond)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", []string{"hi"}, RunConfig{
		Command: "echo",
		Args:    []string{"hi"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = pr.Start(ctx, fetcher)

	// Trigger onConfigChange directly.
	pr.onConfigChange(map[string]string{"A": "2"})

	pr.Stop()
}

func TestProcessRestarterStopProcess(t *testing.T) {
	t.Parallel()

	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", nil, RunConfig{})

	// stopProcess with nil process — should be a no-op.
	pr.stopProcess()
	pr.cleanup()
}

func TestConfigWatcherCheckConfigNoChange(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{"A": "1"})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)

	var calls atomic.Int32
	cw.AddCallback(func(_ map[string]string) { calls.Add(1) })

	// First: detects change (initial hash 0 → new hash X).
	cw.checkConfig()
	time.Sleep(20 * time.Millisecond) // callbacks are async

	// Second: same config, hash unchanged → early return at line 94-97.
	cw.checkConfig()
	time.Sleep(20 * time.Millisecond)

	if calls.Load() != 1 {
		t.Fatalf("expected 1 callback call (only on first change), got %d", calls.Load())
	}
}

func TestProcessRestarterStopNotRunning(t *testing.T) {
	t.Parallel()
	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", nil, RunConfig{})

	// Stop without Start — should be a no-op (not running).
	pr.Stop()
}

func TestProcessRestarterOnConfigChangeError(t *testing.T) {
	t.Parallel()
	lg := &testLogger{}
	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, lg, time.Second)
	pr := NewProcessRestarter(cw, lg, "__nonexistent__", nil, RunConfig{
		Command: "__nonexistent__",
	})

	// onConfigChange calls startProcess which will fail for a nonexistent command.
	pr.onConfigChange(map[string]string{"A": "new"})

	if len(lg.errors) == 0 {
		t.Fatal("expected error logged when startProcess fails in onConfigChange")
	}
}

func TestProcessRestarterStopProcessNonNil(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix process required")
	}

	fetcher := newWatcherFetcher(map[string]string{})
	cw := NewConfigWatcher(fetcher, &testLogger{}, time.Second)
	pr := NewProcessRestarter(cw, &testLogger{}, "echo", nil, RunConfig{})

	// Start and immediately reap a process, then inject into pr.process.
	cmd := exec.Command("echo", "hi")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	process := cmd.Process
	// Don't wait — just inject the process as non-nil (may still be running).
	pr.processMu.Lock()
	pr.process = process
	pr.processMu.Unlock()

	// stopProcess should signal and clear the process.
	pr.stopProcess()

	pr.processMu.Lock()
	nilAfter := pr.process == nil
	pr.processMu.Unlock()

	if !nilAfter {
		t.Fatal("expected process to be nil after stopProcess")
	}
	// Clean up the process.
	_, _ = process.Wait()
}
