package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

var execCommand = exec.Command
var syscallKill = syscall.Kill

// testLogger records log calls for assertion in tests.
type testLogger struct {
	debugs   []string
	infos    []string
	warnings []string
	errors   []string
}

func (tl *testLogger) Debug(msg string)            { tl.debugs = append(tl.debugs, msg) }
func (tl *testLogger) Info(msg string)             { tl.infos = append(tl.infos, msg) }
func (tl *testLogger) Warning(msg string)          { tl.warnings = append(tl.warnings, msg) }
func (tl *testLogger) Error(msg string, err error) { tl.errors = append(tl.errors, msg) }

// fakeFetcher is a test-only runner.EnvFetcher.
type fakeFetcher struct {
	config map[string]string
	err    error
	calls  atomic.Int32
}

func (f *fakeFetcher) Fetch(_ context.Context) (map[string]string, error) {
	f.calls.Add(1)
	return f.config, f.err
}

// fakeSSESubscriber calls onConfig once with the configured map then blocks
// until ctx is cancelled.
type fakeSSESubscriber struct {
	config map[string]string
}

func (f *fakeSSESubscriber) Stream(ctx context.Context, onConfig func(map[string]string)) error {
	onConfig(f.config)
	<-ctx.Done()
	return ctx.Err()
}

// fakeVersionStore records the most recent Store call for assertion.
type fakeVersionStore struct {
	lastStored atomic.Int64
	closed     atomic.Bool
}

func (f *fakeVersionStore) Store(v int64) { f.lastStored.Store(v) }
func (f *fakeVersionStore) Close() error  { f.closed.Store(true); return nil }

func newTestAgent(cfg AgentConfig) (*Agent, *fakeFetcher, *fakeSSESubscriber, *fakeVersionStore, *testLogger) {
	fetcher := &fakeFetcher{config: map[string]string{"TEST": "value"}}
	sse := &fakeSSESubscriber{config: map[string]string{"SSE_KEY": "SSE_VALUE"}}
	vs := &fakeVersionStore{}
	lg := &testLogger{}
	a := NewAgent(cfg, fetcher, sse, vs, lg)
	return a, fetcher, sse, vs, lg
}

func TestNewAgent(t *testing.T) {
	t.Parallel()

	cfg := AgentConfig{
		HubURL:       "http://test.com",
		Token:        "test-token",
		ServiceName:  "test-service",
		PollInterval: 10,
		Verbose:      true,
	}

	a, _, _, _, _ := newTestAgent(cfg)

	if a.config.HubURL != "http://test.com" {
		t.Fatalf("expected HubURL to be set, got %s", a.config.HubURL)
	}
	if a.config.ServiceName != "test-service" {
		t.Fatalf("expected ServiceName to be set, got %s", a.config.ServiceName)
	}
	expectedConfigFile := filepath.Join(shmDir(), "neith-test-service.json")
	if a.configFile != expectedConfigFile {
		t.Fatalf("expected config file %s, got %s", expectedConfigFile, a.configFile)
	}
}

func TestNewAgentDefaults(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{})

	if a.config.HubURL != DefaultHubURL {
		t.Fatalf("expected default HubURL %s, got %s", DefaultHubURL, a.config.HubURL)
	}
	if a.config.PollInterval != DefaultPollInterval {
		t.Fatalf("expected default PollInterval %d, got %d", DefaultPollInterval, a.config.PollInterval)
	}
	if a.config.ServiceName != "default" {
		t.Fatalf("expected default ServiceName 'default', got %s", a.config.ServiceName)
	}
}

func TestAgentGetCurrentConfig(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})

	if len(a.GetCurrentConfig()) != 0 {
		t.Fatalf("expected empty initial config")
	}

	a.mu.Lock()
	a.snapshot = map[string]string{"TEST": "VALUE", "OTHER": "DATA"}
	a.mu.Unlock()

	got := a.GetCurrentConfig()
	if len(got) != 2 || got["TEST"] != "VALUE" {
		t.Fatalf("expected TEST=VALUE in config, got %v", got)
	}
}

func TestAgentGetCurrentVersion(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})

	if a.GetCurrentVersion() != 0 {
		t.Fatalf("expected initial version 0, got %d", a.GetCurrentVersion())
	}

	a.version.Add(5)

	if a.GetCurrentVersion() != 5 {
		t.Fatalf("expected version 5, got %d", a.GetCurrentVersion())
	}
}

func TestAgentConfigChanged(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})

	if a.configChanged(map[string]string{}) {
		t.Fatal("expected no change for empty vs empty")
	}

	a.mu.Lock()
	a.snapshot = map[string]string{"A": "1", "B": "2"}
	a.mu.Unlock()

	if a.configChanged(map[string]string{"A": "1", "B": "2"}) {
		t.Fatal("expected no change for identical configs")
	}
	if !a.configChanged(map[string]string{"A": "1", "B": "3"}) {
		t.Fatal("expected change for different value")
	}
	if !a.configChanged(map[string]string{"A": "1", "B": "2", "C": "3"}) {
		t.Fatal("expected change for additional key")
	}
}

func TestAgentWriteConfigAtomically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	testConfig := map[string]string{
		"KEY1": "VALUE1",
		"KEY2": "VALUE2",
	}

	if err := a.writeConfigAtomically(testConfig); err != nil {
		t.Fatalf("writeConfigAtomically failed: %v", err)
	}

	content, err := os.ReadFile(a.configFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["KEY1"] != "VALUE1" || parsed["KEY2"] != "VALUE2" {
		t.Fatalf("unexpected parsed config: %v", parsed)
	}
}

func TestAgentWriteConfigAtomicallySpecialChars(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	testConfig := map[string]string{
		"QUOTE": `val"ue`,
		"SLASH": `val\ue`,
		"NEWLN": "val\nue",
	}

	if err := a.writeConfigAtomically(testConfig); err != nil {
		t.Fatalf("writeConfigAtomically failed: %v", err)
	}

	content, err := os.ReadFile(a.configFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("unmarshal special-char config: %v", err)
	}
	if parsed["QUOTE"] != `val"ue` || parsed["SLASH"] != `val\ue` || parsed["NEWLN"] != "val\nue" {
		t.Fatalf("special chars not preserved: %v", parsed)
	}
}

func TestAgentWriteConfigAtomicallyError(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = "/nonexistent/dir/config.json"

	if err := a.writeConfigAtomically(map[string]string{"K": "V"}); err == nil {
		t.Fatal("expected error writing to nonexistent dir")
	}
}

func TestAgentWriteVersionCounter(t *testing.T) {
	t.Parallel()

	a, _, _, vs, _ := newTestAgent(AgentConfig{ServiceName: "test"})

	a.writeVersionCounter(42)

	if got := vs.lastStored.Load(); got != 42 {
		t.Fatalf("expected version 42 stored, got %d", got)
	}
}

func TestAgentApplyConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, vs, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	a.applyConfig(map[string]string{"K": "V"})

	if a.GetCurrentVersion() != 1 {
		t.Fatalf("expected version 1 after apply, got %d", a.GetCurrentVersion())
	}
	if vs.lastStored.Load() != 1 {
		t.Fatalf("expected verStore.Store(1), got %d", vs.lastStored.Load())
	}
	if _, err := os.Stat(a.configFile); os.IsNotExist(err) {
		t.Fatal("config file not written")
	}
}

func TestAgentApplyConfigNoChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	a.applyConfig(map[string]string{"K": "V"})
	v1 := a.GetCurrentVersion()

	// Same config — no change, version must not increment
	a.applyConfig(map[string]string{"K": "V"})
	if a.GetCurrentVersion() != v1 {
		t.Fatalf("expected version unchanged at %d, got %d", v1, a.GetCurrentVersion())
	}
}

func TestAgentApplyConfigWriteError(t *testing.T) {
	t.Parallel()

	a, _, _, _, lg := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = "/nonexistent/dir/config.json"

	a.applyConfig(map[string]string{"K": "V"})

	// Version is incremented before the file write — check that error was logged
	if len(lg.errors) == 0 {
		t.Fatal("expected an error to be logged when config write fails")
	}
}

func TestAgentRunSSEStream(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test-service", PollInterval: 1})
	a.configFile = filepath.Join(dir, "config.json")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a.runSSEStream(ctx)

	if a.GetCurrentVersion() == 0 {
		t.Fatal("expected SSE stream to deliver at least one config update")
	}
	if _, err := os.Stat(a.configFile); os.IsNotExist(err) {
		t.Fatal("expected config file to be created by SSE update")
	}
}

func TestAgentRunJitterPolling(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test-service", PollInterval: 1})
	a.configFile = filepath.Join(dir, "config.json")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a.runJitterPolling(ctx)

	if a.GetCurrentVersion() == 0 {
		t.Fatal("expected at least one jitter poll to deliver a config update")
	}
}

func TestAgentFetchAndUpdateConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	a.fetchAndUpdateConfig(context.Background())

	if a.GetCurrentVersion() == 0 {
		t.Fatal("expected version to be incremented")
	}
	if len(a.GetCurrentConfig()) == 0 {
		t.Fatal("expected config data after fetch")
	}
	if _, err := os.Stat(a.configFile); os.IsNotExist(err) {
		t.Fatal("expected config file to be created")
	}
}

func TestAgentFetchAndUpdateConfigError(t *testing.T) {
	t.Parallel()

	a, fetcher, _, _, lg := newTestAgent(AgentConfig{ServiceName: "test"})
	fetcher.err = fmt.Errorf("simulated fetch error") //nolint
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	a.fetchAndUpdateConfig(context.Background())

	if a.GetCurrentVersion() != 0 {
		t.Fatal("expected no version increment on fetch error")
	}
	if len(lg.errors) == 0 {
		t.Fatal("expected error to be logged")
	}
}

func TestAgentFetchAndUpdateConfigNoChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = filepath.Join(dir, "config.json")

	a.fetchAndUpdateConfig(context.Background())
	v1 := a.GetCurrentVersion()

	a.fetchAndUpdateConfig(context.Background()) // same config returned by fakeFetcher
	if a.GetCurrentVersion() != v1 {
		t.Fatalf("expected no version increment for unchanged config: got %d, want %d", a.GetCurrentVersion(), v1)
	}
}


func TestAgentGetEnvOrDefault(t *testing.T) {
	result := getEnvOrDefaultTestHelper("NON_EXISTENT_VAR", "default")
	if result != "default" {
		t.Fatalf("expected default, got %s", result)
	}

	t.Setenv("TEST_VAR", "test_value")
	result = getEnvOrDefaultTestHelper("TEST_VAR", "default")
	if result != "test_value" {
		t.Fatalf("expected env value, got %s", result)
	}
}

func getEnvOrDefaultTestHelper(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}

func TestAgentShmDir(t *testing.T) {
	t.Parallel()
	dir := ShmDir()
	if dir == "" {
		t.Fatal("expected non-empty ShmDir()")
	}
}

func TestAgentRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a, _, _, vs, _ := newTestAgent(AgentConfig{ServiceName: "run-test", PollInterval: 1})
	a.configFile = filepath.Join(dir, "config.json")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop within 3s after cancel()")
	}

	if !vs.closed.Load() {
		t.Fatal("expected verStore.Close() to be called on shutdown")
	}
}

func TestAgentRunRequiresInitialConfig(t *testing.T) {
	t.Parallel()

	fetcher := &fakeFetcher{err: fmt.Errorf("bootstrap unavailable")}
	verStore := &fakeVersionStore{}
	a := NewAgent(
		AgentConfig{ServiceName: "bootstrap-test", Command: "echo", Args: []string{"must-not-run"}},
		fetcher,
		&fakeSSESubscriber{},
		verStore,
		&testLogger{},
	)
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := a.Run(ctx); err == nil {
		t.Fatal("expected bootstrap failure before child or background workers start")
	}
	if got := fetcher.calls.Load(); got != 1 {
		t.Fatalf("expected exactly one synchronous bootstrap fetch, got %d", got)
	}
	if !verStore.closed.Load() {
		t.Fatal("expected version store to close after bootstrap failure")
	}
}

func TestAgentRunWithChild(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName:  "run-child-test",
		PollInterval: 1,
		Command:      "echo",
		Args:         []string{"hello"},
		StopDelay:    10 * time.Millisecond,
	})
	a.configFile = filepath.Join(dir, "config.json")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() with child returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() with child did not stop in time")
	}
}

func TestAgentRunJitterPollingPreCancelled(t *testing.T) {
	t.Parallel()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test", PollInterval: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.runJitterPolling(ctx) // must return immediately
}

func TestAgentWriteConfigAtomicallyRenameError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.osRename = func(_, _ string) error { return fmt.Errorf("rename error") }
	a.configFile = filepath.Join(dir, "config.json")

	if err := a.writeConfigAtomically(map[string]string{"K": "V"}); err == nil {
		t.Fatal("expected rename error")
	}
}

func TestAgentStartChildProcess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "test",
		Command:     "echo",
		Args:        []string{"hello"},
	})
	a.configFile = filepath.Join(dir, "config.json")

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess: %v", err)
	}
	a.waitForChildProcess()
}

func TestAgentStartChildProcessAlreadyRunning(t *testing.T) {
	t.Parallel()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.childMu.Lock()
	a.childProcess = &os.Process{Pid: 99999}
	a.childMu.Unlock()

	if err := a.startChildProcess(); err == nil {
		t.Fatal("expected error for already-running process")
	}
}

func TestAgentStartChildProcessInjectEnv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "test",
		Command:     "echo",
		Args:        []string{"hello"},
		InjectEnv:   true,
	})
	a.configFile = filepath.Join(dir, "config.json")
	a.snapshot = map[string]string{"INJECTED": "yes"}

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess with InjectEnv: %v", err)
	}
	a.waitForChildProcess()
}

func TestAgentWaitForChildProcessNil(t *testing.T) {
	t.Parallel()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.waitForChildProcess() // should return immediately without panic
}

func TestAgentWaitForChildProcess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "test",
		Command:     "echo",
		Args:        []string{"done"},
	})
	a.configFile = filepath.Join(dir, "config.json")

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess: %v", err)
	}
	a.waitForChildProcess() // should complete cleanly
}

func TestAgentStopChildProcess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM not available on windows")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "test",
		Command:     "sleep",
		Args:        []string{"10"},
		StopDelay:   10 * time.Millisecond,
	})
	a.configFile = filepath.Join(dir, "config.json")

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.stopChildProcess()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stopChildProcess did not return within 2s")
	}
}

func TestAgentWriteConfigAtomicallyMarshalError(t *testing.T) {
	t.Parallel()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "test"})
	a.jsonMarshalFn = func(_ any) ([]byte, error) { return nil, fmt.Errorf("marshal error") }
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	if err := a.writeConfigAtomically(map[string]string{"K": "V"}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestAgentStartChildProcessStartError(t *testing.T) {
	t.Parallel()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "test",
		Command:     "__nonexistent_command_xyz__",
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	if err := a.startChildProcess(); err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestAgentWaitForChildProcessWaitError(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix process wait semantics required")
	}
	a, _, _, _, lg := newTestAgent(AgentConfig{ServiceName: "test"})

	// Start and immediately reap the process outside the agent.
	cmd := execCommand("echo", "hello")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	process := cmd.Process
	_, _ = process.Wait() // reap the process

	// Now set the already-waited process — next Wait() will fail.
	a.childMu.Lock()
	a.childProcess = process
	a.childMu.Unlock()

	a.waitForChildProcess()

	if len(lg.errors) == 0 {
		t.Fatal("expected wait error to be logged")
	}
}

func TestAgentStopChildProcessDeadProcess(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix signal semantics required")
	}
	a, _, _, _, lg := newTestAgent(AgentConfig{
		ServiceName: "test",
		StopDelay:   10 * time.Millisecond,
	})

	// Start and reap a process, then inject it as already-dead.
	cmd := execCommand("echo", "hello")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	process := cmd.Process
	_, _ = process.Wait()

	a.childMu.Lock()
	a.childProcess = process
	a.childMu.Unlock()

	a.stopChildProcess()

	// Both SIGTERM and Kill should have failed and been logged.
	if len(lg.errors) < 2 {
		t.Fatalf("expected at least 2 errors (SIGTERM + Kill), got %d: %v", len(lg.errors), lg.errors)
	}
}

func TestAgentRunWithFailingChildStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a, _, _, _, lg := newTestAgent(AgentConfig{
		ServiceName: "test",
		PollInterval: 1,
		Command:     "__nonexistent_xyz__",
		StopDelay:   10 * time.Millisecond,
	})
	a.configFile = filepath.Join(dir, "config.json")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop in time")
	}

	if len(lg.errors) == 0 {
		t.Fatal("expected child start failure to be logged")
	}
}

func TestAgentRunSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM not available on windows")
	}
	dir := t.TempDir()
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "sig-test", PollInterval: 1})
	a.configFile = filepath.Join(dir, "config.json")

	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background()) }()

	time.Sleep(50 * time.Millisecond)
	// Run() registered signal.Notify before we get here; SIGTERM goes to sigCh.
	syscallKill(os.Getpid(), syscall.SIGTERM)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error from Run(): %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop on SIGTERM within 3s")
	}
}
