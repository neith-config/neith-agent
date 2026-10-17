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

// fakeFetcher is a test-only implementation of the live snapshot fetcher; its
// legacy Fetch method remains for older unit-test helpers.
type fakeFetcher struct {
	config map[string]string
	err    error
	calls  atomic.Int32
}

func (f *fakeFetcher) Fetch(_ context.Context) (map[string]string, error) {
	f.calls.Add(1)
	return f.config, f.err
}

func (f *fakeFetcher) FetchSnapshot(_ context.Context) (Snapshot, error) {
	f.calls.Add(1)
	return Snapshot{Config: f.config}, f.err
}

type cancelingFetcher struct {
	config map[string]string
	cancel context.CancelFunc
}

func (f *cancelingFetcher) Fetch(_ context.Context) (map[string]string, error) {
	f.cancel()
	return f.config, nil
}

func (f *cancelingFetcher) FetchSnapshot(_ context.Context) (Snapshot, error) {
	f.cancel()
	return Snapshot{Config: f.config}, nil
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

func (f *fakeSSESubscriber) StreamSnapshots(ctx context.Context, onSnapshot func(Snapshot)) error {
	onSnapshot(Snapshot{Config: f.config})
	<-ctx.Done()
	return ctx.Err()
}

// fakeVersionStore records the most recent Store call for assertion.
type fakeVersionStore struct {
	lastStored atomic.Int64
	current    atomic.Int64
	closed     atomic.Bool
}

func (f *fakeVersionStore) Current() int64 {
	return f.current.Load()
}

func (f *fakeVersionStore) Store(v int64) {
	f.lastStored.Store(v)
	f.current.Store(v)
}
func (f *fakeVersionStore) Close() error { f.closed.Store(true); return nil }

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

func TestAgentWritesConfiguredJSONOutput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "nested", "config.json")
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName:    "json-output",
		WriteJSONFile:  true,
		JSONOutputPath: jsonPath,
	})
	a.configFile = filepath.Join(dir, "internal.json")

	a.applyConfig(map[string]string{"VISIBLE": "value"})

	content, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read configured JSON output: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("parse configured JSON output: %v", err)
	}
	if got["VISIBLE"] != "value" {
		t.Fatalf("unexpected configured JSON output: %v", got)
	}
}

func TestAgentNoJSONOutputStillPublishesCanonicalSnapshot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "application.json")
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName:    "no-json-output",
		JSONOutputPath: jsonPath,
		WriteJSONFile:  false,
	})
	a.configFile = filepath.Join(dir, "internal.json")

	a.applyConfig(map[string]string{"INTERNAL": "value"})

	if _, err := os.Stat(a.configFile); err != nil {
		t.Fatalf("canonical snapshot was not published: %v", err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("secondary JSON output should be suppressed, stat error: %v", err)
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

	if !a.configChanged(map[string]string{}) {
		t.Fatal("expected an uninitialized empty config to be treated as a change")
	}

	a.mu.Lock()
	a.snapshot = map[string]string{"A": "1", "B": "2"}
	a.initialized = true
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

func TestAgentApplyConfigDistinguishesMissingAndEmpty(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "presence"})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	a.applyConfig(map[string]string{"REMOVED": ""})
	a.applyConfig(map[string]string{"ADDED": ""})

	if got := a.GetCurrentVersion(); got != 2 {
		t.Fatalf("expected missing-to-empty key replacement to publish version 2, got %d", got)
	}
	config := a.GetCurrentConfig()
	if _, ok := config["REMOVED"]; ok {
		t.Fatalf("removed key remained in config: %v", config)
	}
	if value, ok := config["ADDED"]; !ok || value != "" {
		t.Fatalf("present empty key missing from config: %v", config)
	}
}

func TestAgentAppliesFirstAuthoritativeEmptySnapshot(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "first-empty"})
	a.configFile = filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(a.configFile, []byte(`{"STALE":"secret"}`), 0o600); err != nil {
		t.Fatalf("seed stale config file: %v", err)
	}
	snapshot := Snapshot{
		Revision: mustTestRevision(t, "018f0000-0000-7000-8000-000000000001"),
		Config:   map[string]string{},
	}

	if err := a.applySnapshot(snapshot, false); err != nil {
		t.Fatalf("apply first empty snapshot: %v", err)
	}
	if got := a.GetCurrentVersion(); got != 1 {
		t.Fatalf("expected first empty snapshot to publish version 1, got %d", got)
	}
	content, err := os.ReadFile(a.configFile)
	if err != nil {
		t.Fatalf("read first empty snapshot file: %v", err)
	}
	var config map[string]string
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("parse first empty snapshot file: %v", err)
	}
	if len(config) != 0 {
		t.Fatalf("expected stale config to be replaced by empty snapshot, got %v", config)
	}
}

func TestAgentEquivalentSnapshotsAreIdempotent(t *testing.T) {
	t.Parallel()

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "idempotent"})
	a.configFile = filepath.Join(t.TempDir(), "config.json")
	first := Snapshot{
		Revision: mustTestRevision(t, "018f0000-0000-7000-8000-000000000001"),
		Config:   map[string]string{"KEY": "value"},
	}
	newerEquivalent := Snapshot{
		Revision: mustTestRevision(t, "018f0000-0000-7000-8000-000000000002"),
		Config:   map[string]string{"KEY": "value"},
	}

	if err := a.applySnapshot(first, false); err != nil {
		t.Fatalf("apply first snapshot: %v", err)
	}
	if err := a.applySnapshot(newerEquivalent, false); err != nil {
		t.Fatalf("apply equivalent newer snapshot: %v", err)
	}
	if err := a.applySnapshot(newerEquivalent, false); err != nil {
		t.Fatalf("apply repeated equivalent snapshot: %v", err)
	}
	if got := a.GetCurrentVersion(); got != 1 {
		t.Fatalf("expected equivalent snapshots to publish once, got version %d", got)
	}
	if a.acceptedRevision == nil || a.acceptedRevision.Compare(newerEquivalent.Revision) != 0 {
		t.Fatalf("expected newer equivalent revision to be accepted, got %v", a.acceptedRevision)
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

func TestAgentContinuesPublicationVersionAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version")
	openStore := func() VersionStore {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			t.Fatalf("open version file: %v", err)
		}
		store, err := NewVersionStore(f)
		if err != nil {
			_ = f.Close()
			t.Fatalf("NewVersionStore: %v", err)
		}
		return store
	}

	firstStore := openStore()
	first := NewAgent(AgentConfig{ServiceName: "restart-one"}, nil, nil, firstStore, &testLogger{})
	first.configFile = filepath.Join(t.TempDir(), "first.json")
	first.applyConfig(map[string]string{"VERSION": "first"})
	if got := first.GetCurrentVersion(); got != 1 {
		t.Fatalf("expected first publication version 1, got %d", got)
	}
	if err := firstStore.Close(); err != nil {
		t.Fatalf("close first version store: %v", err)
	}

	secondStore := openStore()
	second := NewAgent(AgentConfig{ServiceName: "restart-two"}, nil, nil, secondStore, &testLogger{})
	second.configFile = filepath.Join(t.TempDir(), "second.json")
	second.applyConfig(map[string]string{"VERSION": "second"})
	if got := second.GetCurrentVersion(); got != 2 {
		t.Fatalf("expected reopened publication version 2, got %d", got)
	}
	if got := secondStore.Current(); got != 2 {
		t.Fatalf("expected durable publication version 2, got %d", got)
	}
	if err := secondStore.Close(); err != nil {
		t.Fatalf("close second version store: %v", err)
	}
}

func TestAgentRejectsStaleOrEqualSnapshotRevision(t *testing.T) {
	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "revision-order"})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	newer := Snapshot{
		Revision: mustTestRevision(t, "018f0000-0000-7000-8000-000000000002"),
		Config:   map[string]string{"VALUE": "new"},
	}
	older := Snapshot{
		Revision: mustTestRevision(t, "018f0000-0000-7000-8000-000000000001"),
		Config:   map[string]string{"VALUE": "old"},
	}

	if err := a.applySnapshot(newer, true); err != nil {
		t.Fatalf("apply newer snapshot: %v", err)
	}
	if err := a.applySnapshot(older, false); err != nil {
		t.Fatalf("apply older snapshot: %v", err)
	}
	if got := a.GetCurrentConfig()["VALUE"]; got != "new" {
		t.Fatalf("stale snapshot replaced newer value: %q", got)
	}
	if got := a.GetCurrentVersion(); got != 1 {
		t.Fatalf("stale snapshot advanced version to %d", got)
	}

	equal := Snapshot{
		Revision: newer.Revision,
		Config:   map[string]string{"VALUE": "equal-revision-different-payload"},
	}
	if err := a.applySnapshot(equal, false); err != nil {
		t.Fatalf("apply equal snapshot: %v", err)
	}
	if got := a.GetCurrentConfig()["VALUE"]; got != "new" {
		t.Fatalf("equal revision replaced accepted value: %q", got)
	}
	if got := a.GetCurrentVersion(); got != 1 {
		t.Fatalf("equal revision advanced version to %d", got)
	}
}

func mustTestRevision(t *testing.T, value string) Revision {
	t.Helper()
	revision, err := ParseRevision(value)
	if err != nil {
		t.Fatalf("parse revision %q: %v", value, err)
	}
	return revision
}

func TestAgentApplyConfigWriteError(t *testing.T) {
	t.Parallel()

	a, _, _, _, lg := newTestAgent(AgentConfig{ServiceName: "test"})
	a.configFile = "/nonexistent/dir/config.json"

	a.applyConfig(map[string]string{"K": "V"})

	// Failed publication must not advance the accepted state or version.
	if a.GetCurrentVersion() != 0 || len(a.GetCurrentConfig()) != 0 {
		t.Fatalf("failed publication changed accepted state: version=%d config=%v", a.GetCurrentVersion(), a.GetCurrentConfig())
	}
	if len(lg.errors) == 0 {
		t.Fatal("expected an error to be logged when config write fails")
	}
}

func TestAgentApplyConfigRetriesAfterPublicationFailure(t *testing.T) {
	dir := t.TempDir()
	a, _, _, vs, _ := newTestAgent(AgentConfig{ServiceName: "retry-publication"})
	a.configFile = filepath.Join(dir, "config.json")

	a.applyConfig(map[string]string{"OLD": "value"})
	if a.GetCurrentVersion() != 1 {
		t.Fatalf("expected initial version 1, got %d", a.GetCurrentVersion())
	}

	attempts := 0
	a.osRename = func(oldPath, newPath string) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("injected rename failure")
		}
		return os.Rename(oldPath, newPath)
	}
	update := map[string]string{"NEW": "value"}
	a.applyConfig(update)

	if a.GetCurrentVersion() != 1 || a.GetCurrentConfig()["OLD"] != "value" {
		t.Fatalf("failed publication changed accepted state: version=%d config=%v", a.GetCurrentVersion(), a.GetCurrentConfig())
	}
	if got := vs.lastStored.Load(); got != 1 {
		t.Fatalf("failed publication changed stored version: got %d", got)
	}

	a.applyConfig(update)

	if attempts != 2 {
		t.Fatalf("expected retry after identical update, got %d rename attempts", attempts)
	}
	if a.GetCurrentVersion() != 2 || a.GetCurrentConfig()["NEW"] != "value" {
		t.Fatalf("retry did not commit accepted state: version=%d config=%v", a.GetCurrentVersion(), a.GetCurrentConfig())
	}
	if got := vs.lastStored.Load(); got != 2 {
		t.Fatalf("retry did not publish version 2: got %d", got)
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

func TestAgentConfigUpdateDoesNotRestartChild(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "no-restart-test",
		Command:     "sleep",
		Args:        []string{"10"},
		StopDelay:   10 * time.Millisecond,
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess: %v", err)
	}
	waitDone := make(chan struct{})
	go func() {
		a.waitForChildProcess()
		close(waitDone)
	}()
	t.Cleanup(func() {
		a.stopChildProcess()
		<-waitDone
	})

	a.childMu.Lock()
	initialPID := a.childProcess.Pid
	a.childMu.Unlock()

	a.applyConfig(map[string]string{"UPDATED": "yes"})

	a.childMu.Lock()
	updatedPID := a.childProcess.Pid
	a.childMu.Unlock()
	if updatedPID != initialPID {
		t.Fatalf("configuration update restarted child: initial PID %d, updated PID %d", initialPID, updatedPID)
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

	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected child start failure to be returned")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after child start failure")
	}

	if len(lg.errors) == 0 {
		t.Fatal("expected child start failure to be logged")
	}
}

func TestAgentRunPropagatesChildExit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "exit-test",
		Command:     "sh",
		Args:        []string{"-c", "exit 42"},
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	done := make(chan error, 1)
	go func() { done <- a.Run(context.Background()) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected child exit status to be returned")
		}
		exitErr, ok := err.(*ChildExitError)
		if !ok {
			t.Fatalf("expected ChildExitError, got %T: %v", err, err)
		}
		if exitErr.Code != 42 {
			t.Fatalf("expected child exit code 42, got %d", exitErr.Code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not return after child exit")
	}
}

func TestAgentRunCancellationBeforeChildLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	marker := filepath.Join(t.TempDir(), "started")
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "cancel-before-launch",
		Command:     "sh",
		Args:        []string{"-c", fmt.Sprintf("printf started > %s; sleep 60", marker)},
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.Run(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child launched after cancellation; marker stat error: %v", err)
	}
}

func TestAgentRunCancellationBetweenBootstrapAndChildLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	marker := filepath.Join(t.TempDir(), "started")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "cancel-during-launch",
		Command:     "sh",
		Args:        []string{"-c", fmt.Sprintf("printf started > %s; sleep 60", marker)},
	})
	a.pollClient = &cancelingFetcher{config: map[string]string{"TEST": "value"}, cancel: cancel}
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	err := a.Run(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child launched after bootstrap cancellation; marker stat error: %v", err)
	}
}

func TestAgentRunCancellationDuringChildLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	marker := filepath.Join(t.TempDir(), "started")
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "cancel-during-start",
		Command:     "sh",
		Args:        []string{"-c", fmt.Sprintf("printf started > %s; sleep 60", marker)},
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	entered := make(chan struct{})
	release := make(chan struct{})
	a.startProcess = func(cmd *exec.Cmd) error {
		close(entered)
		<-release
		return context.Canceled
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("child launch did not reach the controlled start point")
	}
	cancel()
	close(release)

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop after cancellation during child launch")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child launched after cancellation during start; marker stat error: %v", err)
	}
}

func TestAgentRunCancellationAfterChildLaunch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child process test uses unix commands")
	}

	marker := filepath.Join(t.TempDir(), "started")
	a, _, _, _, _ := newTestAgent(AgentConfig{
		ServiceName: "cancel-after-launch",
		Command:     "sh",
		Args:        []string{"-c", fmt.Sprintf("printf started > %s; sleep 60", marker)},
		StopDelay:   100 * time.Millisecond,
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()

	if err := waitForTestFile(marker, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() returned unexpected cancellation error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}

	a.childMu.Lock()
	defer a.childMu.Unlock()
	if a.childProcess != nil {
		t.Fatalf("child process remained tracked after cancellation: %d", a.childProcess.Pid)
	}
}

func waitForTestFile(path string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return fmt.Errorf("file %s was not created", path)
		}
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
