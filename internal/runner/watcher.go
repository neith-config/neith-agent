package runner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neith-config/neith-agent/internal/logger"
)

// ConfigWatcher monitors configuration changes and triggers callbacks
type ConfigWatcher struct {
	fetcher   EnvFetcher
	logger    logger.Logger
	interval  time.Duration
	lastHash  atomic.Uint64
	callbacks []func(map[string]string)
	ctx       context.Context
	cancel    context.CancelFunc
	running   atomic.Bool
	mu        sync.RWMutex
}

// NewConfigWatcher creates a new configuration watcher
func NewConfigWatcher(fetcher EnvFetcher, lg logger.Logger, interval time.Duration) *ConfigWatcher {
	return &ConfigWatcher{
		fetcher:  fetcher,
		logger:   lg,
		interval: interval,
		ctx:      context.Background(),
		cancel:   func() {},
	}
}

// AddCallback adds a callback function to be called when config changes
func (cw *ConfigWatcher) AddCallback(callback func(map[string]string)) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.callbacks = append(cw.callbacks, callback)
}

// Start begins watching for configuration changes
func (cw *ConfigWatcher) Start(ctx context.Context) {
	if !cw.running.CompareAndSwap(false, true) {
		return // Already running
	}

	cw.ctx, cw.cancel = context.WithCancel(ctx)
	go cw.watchLoop()
}

// Stop stops the configuration watcher
func (cw *ConfigWatcher) Stop() {
	if !cw.running.CompareAndSwap(true, false) {
		return // Not running
	}

	cw.cancel()
}

// watchLoop runs the main watching loop
func (cw *ConfigWatcher) watchLoop() {
	ticker := time.NewTicker(cw.interval)
	defer ticker.Stop()

	// Initial fetch
	cw.checkConfig()

	for {
		select {
		case <-cw.ctx.Done():
			return
		case <-ticker.C:
			cw.checkConfig()
		}
	}
}

// checkConfig fetches current config and triggers callbacks if changed
func (cw *ConfigWatcher) checkConfig() {
	ctx, cancel := context.WithTimeout(cw.ctx, 10*time.Second)
	defer cancel()

	config, err := cw.fetcher.Fetch(ctx)
	if err != nil {
		cw.logger.Error("Failed to fetch config for watching", err)
		return
	}

	hash := cw.hashConfig(config)
	if cw.lastHash.CompareAndSwap(hash, hash) {
		// Hash is the same, no change
		return
	}

	// Hash changed, update and trigger callbacks
	cw.lastHash.Store(hash)
	cw.logger.Info("Configuration change detected")

	cw.mu.RLock()
	callbacks := make([]func(map[string]string), len(cw.callbacks))
	copy(callbacks, cw.callbacks)
	cw.mu.RUnlock()

	// Call all callbacks
	for _, callback := range callbacks {
		go callback(config)
	}
}

// hashConfig creates a simple hash of the configuration map
func (cw *ConfigWatcher) hashConfig(config map[string]string) uint64 {
	var hash uint64 = 5381
	for k, v := range config {
		// Simple hash combining keys and values
		for _, c := range k {
			hash = ((hash << 5) + hash) + uint64(c)
		}
		for _, c := range v {
			hash = ((hash << 5) + hash) + uint64(c)
		}
	}
	return hash
}

// ProcessRestarter manages child process restarts on config changes
type ProcessRestarter struct {
	watcher   *ConfigWatcher
	logger    logger.Logger
	cmd       string
	args      []string
	runConfig RunConfig
	process   *os.Process
	processMu sync.Mutex
	running   atomic.Bool
	stopCh    chan struct{}
}

// NewProcessRestarter creates a new process restarter
func NewProcessRestarter(watcher *ConfigWatcher, lg logger.Logger, cmd string, args []string, cfg RunConfig) *ProcessRestarter {
	return &ProcessRestarter{
		watcher:   watcher,
		logger:    lg,
		cmd:       cmd,
		args:      args,
		runConfig: cfg,
		stopCh:    make(chan struct{}),
	}
}

// Start begins monitoring and restarts process on config changes
func (pr *ProcessRestarter) Start(ctx context.Context, initialFetcher EnvFetcher) error {
	if !pr.running.CompareAndSwap(false, true) {
		return nil // Already running
	}

	// Start initial process
	if err := pr.startProcess(ctx, initialFetcher); err != nil {
		return err
	}

	// Add callback for config changes
	pr.watcher.AddCallback(pr.onConfigChange)

	// Start watcher
	pr.watcher.Start(ctx)

	// Wait for stop signal
	go func() {
		<-pr.stopCh
		pr.cleanup()
	}()

	return nil
}

// Stop stops the process restarter
func (pr *ProcessRestarter) Stop() {
	if !pr.running.CompareAndSwap(true, false) {
		return // Not running
	}

	close(pr.stopCh)
	pr.watcher.Stop()
	pr.stopProcess()
}

// onConfigChange is called when configuration changes
func (pr *ProcessRestarter) onConfigChange(config map[string]string) {
	pr.logger.Info("Restarting process due to configuration change")

	// Stop current process
	pr.stopProcess()

	// Start new process with updated config
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := pr.startProcess(ctx, pr.watcher.fetcher); err != nil {
		pr.logger.Error("Failed to restart process", err)
	}
}

// startProcess starts a new child process
func (pr *ProcessRestarter) startProcess(ctx context.Context, fetcher EnvFetcher) error {
	pr.processMu.Lock()
	defer pr.processMu.Unlock()

	exitCode := Run(ctx, fetcher, pr.logger, pr.runConfig)
	if exitCode != 0 {
		return &ProcessError{ExitCode: exitCode, Cmd: pr.cmd}
	}
	return nil
}

// ProcessError represents an error from child process execution
type ProcessError struct {
	ExitCode int
	Cmd      string
}

func (pe *ProcessError) Error() string {
	return fmt.Sprintf("process %q exited with code %d", pe.Cmd, pe.ExitCode)
}

// stopProcess stops the current child process
func (pr *ProcessRestarter) stopProcess() {
	pr.processMu.Lock()
	defer pr.processMu.Unlock()

	if pr.process != nil {
		pr.process.Signal(os.Interrupt)
		pr.process = nil
	}
}

// cleanup performs final cleanup
func (pr *ProcessRestarter) cleanup() {
	pr.stopProcess()
}
