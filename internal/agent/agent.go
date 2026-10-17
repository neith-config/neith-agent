package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/neith-config/neith-agent/internal/logger"
	"github.com/neith-config/neith-agent/internal/runner"
)

const (
	DefaultHubURL       = "http://localhost:8080"
	DefaultPollInterval = 5 // seconds
	DefaultStopDelay    = 5 * time.Second
	ConfigFileName      = "neith-%s.json"
	VersionFileName     = "neith-%s.version"
)


// SSESubscriber streams config updates from a remote source. HubSSEClient
// implements this interface; tests inject a fake.
type SSESubscriber interface {
	Stream(ctx context.Context, onConfig func(map[string]string)) error
}

// VersionStore writes a version counter to a persistent backing store.
// NewVersionStore returns the platform-specific implementation; tests inject a fake.
type VersionStore interface {
	Store(version int64)
	Close() error
}

// ShmDir returns the base directory used for shared-memory files.
func ShmDir() string { return shmDir() }

// ChildExitError reports a non-zero exit status from the supervised child.
type ChildExitError struct {
	Code int
}

func (e *ChildExitError) Error() string {
	return fmt.Sprintf("child process exited with status %d", e.Code)
}

type Agent struct {
	config     AgentConfig
	pollClient runner.EnvFetcher
	sseClient  SSESubscriber
	verStore   VersionStore
	logger     logger.Logger
	version    atomic.Int64
	configFile string
	mu           sync.RWMutex
	snapshot     map[string]string
	osRename     func(string, string) error
	osRemove     func(string) error
	jsonMarshalFn func(any) ([]byte, error)

	childCmd     string
	childArgs    []string
	childProcess     *os.Process
	childMu          sync.Mutex
	childDone        chan struct{}
	childWaitStarted bool
	startProcess     func(*exec.Cmd) error
}

type AgentConfig struct {
	HubURL       string
	Token        string
	ServiceName  string
	PollInterval int
	Verbose      bool
	Command      string
	Args         []string
	InjectEnv    bool
	StopDelay    time.Duration // graceful child shutdown wait; defaults to DefaultStopDelay
}

// NewAgent creates an Agent. All dependencies are passed in; the constructor
// does not create any resources itself.
func NewAgent(cfg AgentConfig, pollClient runner.EnvFetcher, sseClient SSESubscriber, verStore VersionStore, lg logger.Logger) *Agent {
	if cfg.HubURL == "" {
		cfg.HubURL = DefaultHubURL
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = DefaultPollInterval
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "default"
	}
	if cfg.StopDelay == 0 {
		cfg.StopDelay = DefaultStopDelay
	}

	configFile := filepath.Join(shmDir(), fmt.Sprintf(ConfigFileName, cfg.ServiceName))

	return &Agent{
		config:        cfg,
		pollClient:    pollClient,
		sseClient:     sseClient,
		verStore:      verStore,
		logger:        lg,
		configFile:    configFile,
		snapshot:      make(map[string]string),
		childCmd:      cfg.Command,
		childArgs:     cfg.Args,
		childDone:     make(chan struct{}),
		startProcess:  func(cmd *exec.Cmd) error { return cmd.Start() },
		osRename:      os.Rename,
		osRemove:      os.Remove,
		jsonMarshalFn: json.Marshal,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer a.verStore.Close()

	a.logger.Info("Starting neith-agent daemon")
	a.logger.Info("Hub URL: " + a.config.HubURL)
	a.logger.Info("Service: " + a.config.ServiceName)
	a.logger.Info("Poll interval: " + fmt.Sprintf("%d seconds", a.config.PollInterval))
	if a.childCmd != "" {
		a.logger.Info("Child command: " + a.childCmd)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	if err := a.bootstrapConfig(ctx); err != nil {
		a.logger.Error("Initial configuration bootstrap failed", err)
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	var childResult chan error
	if a.childCmd != "" {
		if err := a.startChildProcessWithContext(ctx); err != nil {
			a.logger.Error("Failed to start child process", err)
			return err
		}
		childResult = make(chan error, 1)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		a.runSSEStream(ctx)
	}()

	go func() {
		defer wg.Done()
		a.runJitterPolling(ctx)
	}()

	if childResult != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			childResult <- a.waitForChildProcess()
		}()
	}

	var runErr error
	select {
	case sig := <-sigCh:
		a.logger.Info("Received signal: " + sig.String())
	case <-ctx.Done():
		a.logger.Info("Context cancelled")
	case runErr = <-childResult:
		if ctx.Err() != nil {
			runErr = nil
		}
	}

	a.logger.Info("Shutting down...")
	a.stopChildProcess()
	cancel()
	wg.Wait()
	a.logger.Info("Shutdown complete")

	return runErr
}

func (a *Agent) runSSEStream(ctx context.Context) {
	if err := a.sseClient.Stream(ctx, a.applyConfig); err != nil {
		a.logger.Error("SSE stream ended", err)
	}
}

func (a *Agent) runJitterPolling(ctx context.Context) {
	baseInterval := time.Duration(a.config.PollInterval) * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		jitter := time.Duration(rand.Intn(int(baseInterval/2))) - time.Duration(int(baseInterval/4))
		nextPoll := baseInterval + jitter

		a.logger.Debug(fmt.Sprintf("Next jitter poll in %v", nextPoll))

		select {
		case <-time.After(nextPoll):
			a.fetchAndUpdateConfig(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// bootstrapConfig fetches and publishes the first valid snapshot before any
// child or background transport is started.
func (a *Agent) bootstrapConfig(ctx context.Context) error {
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	newConfig, err := a.pollClient.Fetch(fetchCtx)
	if err != nil {
		return fmt.Errorf("fetch initial configuration: %w", err)
	}
	if err := a.applyConfigResult(newConfig, true); err != nil {
		return fmt.Errorf("publish initial configuration: %w", err)
	}
	return nil
}

// applyConfig applies a new config snapshot: detects changes, increments the
// version counter, writes the config file atomically, and updates the mmap
// version store. It is the shared write path for both SSE and jitter polling.
func (a *Agent) applyConfig(newConfig map[string]string) {
	if err := a.applyConfigResult(newConfig, false); err != nil {
		a.logger.Error("Failed to apply configuration", err)
	}
}

func (a *Agent) applyConfigResult(newConfig map[string]string, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !force && !a.configChanged(newConfig) {
		return nil
	}

	if err := a.writeConfigAtomically(newConfig); err != nil {
		return err
	}

	a.snapshot = newConfig
	newVersion := a.version.Add(1)
	a.writeVersionCounter(newVersion)
	a.logger.Info(fmt.Sprintf("Config updated to version %d with %d variables", newVersion, len(newConfig)))
	return nil
}

func (a *Agent) fetchAndUpdateConfig(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	newConfig, err := a.pollClient.Fetch(ctx)
	if err != nil {
		a.logger.Error("Failed to fetch config", err)
		return
	}

	a.applyConfig(newConfig)
}

func (a *Agent) configChanged(newConfig map[string]string) bool {
	if len(a.snapshot) != len(newConfig) {
		return true
	}
	for k, v := range a.snapshot {
		if newConfig[k] != v {
			return true
		}
	}
	return false
}

func (a *Agent) writeConfigAtomically(config map[string]string) error {
	data, err := a.jsonMarshalFn(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tempFile := a.configFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := a.osRename(tempFile, a.configFile); err != nil {
		a.osRemove(tempFile)
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

func (a *Agent) writeVersionCounter(version int64) {
	a.verStore.Store(version)
}

func (a *Agent) GetCurrentConfig() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]string, len(a.snapshot))
	for k, v := range a.snapshot {
		result[k] = v
	}
	return result
}

func (a *Agent) GetCurrentVersion() int64 {
	return a.version.Load()
}

func (a *Agent) startChildProcess() error {
	return a.startChildProcessWithContext(context.Background())
}

func (a *Agent) startChildProcessWithContext(ctx context.Context) error {
	a.childMu.Lock()
	defer a.childMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if a.childProcess != nil {
		return fmt.Errorf("child process already running")
	}

	config := a.GetCurrentConfig()

	var env []string
	if a.config.InjectEnv {
		env = runner.MergeEnv(os.Environ(), config, a.logger)
	} else {
		env = os.Environ()
	}

	cmd := exec.Command(a.childCmd, a.childArgs...)
	configureChildProcess(cmd)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := a.startProcess(cmd); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("failed to start child command: %w", err)
	}

	a.childProcess = cmd.Process
	a.childDone = make(chan struct{})
	a.childWaitStarted = false
	a.logger.Info(fmt.Sprintf("Started child process %d: %s", a.childProcess.Pid, a.childCmd))

	return nil
}

func (a *Agent) waitForChildProcess() error {
	a.childMu.Lock()
	if a.childProcess == nil || a.childWaitStarted {
		a.childMu.Unlock()
		return nil
	}
	a.childWaitStarted = true
	process := a.childProcess
	a.childMu.Unlock()

	state, err := process.Wait()

	a.childMu.Lock()
	a.childProcess = nil
	a.childMu.Unlock()

	close(a.childDone)

	if err != nil {
		a.logger.Error("Child process wait failed", err)
		return fmt.Errorf("wait for child process: %w", err)
	}

	if state == nil {
		return fmt.Errorf("wait for child process: missing process state")
	}

	code := state.ExitCode()
	a.logger.Info(fmt.Sprintf("Child process exited with status: %d", code))
	if code != 0 {
		return &ChildExitError{Code: code}
	}
	return nil
}

func (a *Agent) stopChildProcess() {
	a.childMu.Lock()
	if a.childProcess == nil {
		a.childMu.Unlock()
		return
	}
	process := a.childProcess
	childDone := a.childDone
	waitStarted := a.childWaitStarted
	a.logger.Info(fmt.Sprintf("Stopping child process %d", process.Pid))
	a.childMu.Unlock()

	if !waitStarted {
		go func() { _ = a.waitForChildProcess() }()
	}

	if err := signalChildProcess(process, syscall.SIGTERM); err != nil {
		a.logger.Error("Failed to send SIGTERM to child process group", err)
	}

	timer := time.NewTimer(a.config.StopDelay)
	defer timer.Stop()

	select {
	case <-childDone:
		a.logger.Info("Child process exited gracefully after SIGTERM")
		return
	case <-timer.C:
	}

	a.childMu.Lock()
	process = a.childProcess
	a.childMu.Unlock()
	if process != nil {
		a.logger.Warning(fmt.Sprintf("Child process %d did not exit within %v, sending SIGKILL", process.Pid, a.config.StopDelay))
		if err := killChildProcess(process); err != nil {
			a.logger.Error("Failed to kill child process group", err)
		}
	}
	<-childDone
}
