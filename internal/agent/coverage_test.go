package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateServiceNameContract(t *testing.T) {
	for _, name := range []string{"", "bad/name", "bad name", "bad.name"} {
		if err := ValidateServiceName(name); err == nil {
			t.Errorf("ValidateServiceName(%q) accepted invalid name", name)
		}
	}
	for _, name := range []string{"alpha", "A-1_ok"} {
		if err := ValidateServiceName(name); err != nil {
			t.Errorf("ValidateServiceName(%q): %v", name, err)
		}
	}
}

func TestAgentErrorPaths(t *testing.T) {
	t.Run("invalid polling interval", func(t *testing.T) {
		a, _, _, _, lg := newTestAgent(AgentConfig{PollInterval: 0})
		a.config.PollInterval = 0
		a.runJitterPolling(context.Background())
		if lg.ErrorCount() == 0 {
			t.Fatal("expected invalid interval error")
		}
	})
	t.Run("bootstrap fetch and publish errors", func(t *testing.T) {
		a, fetcher, _, _, _ := newTestAgent(AgentConfig{})
		fetcher.SetError(errors.New("fetch"))
		if err := a.bootstrapConfig(context.Background()); err == nil {
			t.Fatal("expected fetch error")
		}
		a, _, _, _, _ = newTestAgent(AgentConfig{})
		a.jsonMarshalFn = func(any) ([]byte, error) { return nil, errors.New("marshal") }
		if err := a.bootstrapConfig(context.Background()); err == nil {
			t.Fatal("expected publish error")
		}
	})
	t.Run("transport publication error is logged", func(t *testing.T) {
		a, _, _, _, lg := newTestAgent(AgentConfig{})
		a.jsonMarshalFn = func(any) ([]byte, error) { return nil, errors.New("marshal") }
		a.applySnapshotFromTransport(Snapshot{Config: map[string]string{"A": "1"}})
		if lg.ErrorCount() == 0 {
			t.Fatal("expected publication error")
		}
	})
	t.Run("JSON output", func(t *testing.T) {
		a, _, _, _, _ := newTestAgent(AgentConfig{WriteJSONFile: true})
		a.config.JSONOutputPath = filepath.Join(t.TempDir(), "missing", "out.json")
		if err := a.writeJSONOutput(map[string]string{"A": "1"}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAgentChildStartContextAndWaitState(t *testing.T) {
	a, _, _, _, _ := newTestAgent(AgentConfig{Command: "true"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.startChildProcessWithContext(ctx); err == nil {
		t.Fatal("expected cancelled context")
	}
	a.childProcess = &os.Process{}
	if err := a.startChildProcessWithContext(context.Background()); err == nil {
		t.Fatal("expected already-running error")
	}
	a.childProcess = nil
	a.childDone = make(chan struct{})
	a.childWaitStarted = true
	if err := a.waitForChildProcess(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAdditionalContracts(t *testing.T) {
	if (&ChildExitError{Code: 7}).Error() == "" {
		t.Fatal("empty child error")
	}
	a, _, _, _, _ := newTestAgent(AgentConfig{WriteJSONFile: true, JSONOutputPath: t.TempDir()})
	if err := a.writeJSONOutput(map[string]string{"A": "1"}); err == nil {
		t.Fatal("expected JSON output directory error")
	}
	a.configFile = filepath.Join(t.TempDir(), "config.json")
	a.config.JSONOutputPath = t.TempDir()
	if err := a.applySnapshot(Snapshot{Config: map[string]string{"A": "1"}}, true); err == nil {
		t.Fatal("expected snapshot output error")
	}
	if err := a.applyConfigResult(map[string]string{"A": "1"}, true); err == nil {
		t.Fatal("expected config output error")
	}
}

type configFile struct {
	writeFn func([]byte) (int, error)
	closeFn func() error
}

func (f configFile) Write(data []byte) (int, error) { return f.writeFn(data) }
func (f configFile) Close() error                   { return f.closeFn() }

func TestAtomicPublicationErrors(t *testing.T) {
	cases := []struct {
		name string
		file atomicConfigFile
	}{
		{"write", configFile{writeFn: func([]byte) (int, error) { return 0, errors.New("write") }, closeFn: func() error { return nil }}},
		{"short write", configFile{writeFn: func(data []byte) (int, error) { return len(data) - 1, nil }, closeFn: func() error { return nil }}},
		{"close", configFile{writeFn: func(data []byte) (int, error) { return len(data), nil }, closeFn: func() error { return errors.New("close") }}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _, _, _ := newTestAgent(AgentConfig{})
			a.configFile = filepath.Join(t.TempDir(), "config")
			a.openConfigFile = func(string, int, os.FileMode) (atomicConfigFile, error) { return tc.file, nil }
			a.osRemove = func(string) error { return nil }
			if err := a.writeConfigAtomically(map[string]string{"A": "1"}); err == nil {
				t.Fatal("expected publication error")
			}
		})
	}
}

func TestDefaultJSONPathError(t *testing.T) {
	a, _, _, _, _ := newTestAgent(AgentConfig{WriteJSONFile: true})
	a.defaultJSONPath = func() (string, error) { return "", errors.New("config dir") }
	if err := a.writeJSONOutput(map[string]string{"A": "1"}); err == nil {
		t.Fatal("expected path resolution error")
	}
}

func TestChildCompletionAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := childCompletionResult(ctx, errors.New("child")); got != nil {
		t.Fatalf("cancelled child result = %v, want nil", got)
	}
	if got := childCompletionResult(context.Background(), errors.New("child")); got == nil {
		t.Fatal("active child result was discarded")
	}
}

func TestRunChildCompletionAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a, _, _, _, _ := newTestAgent(AgentConfig{Command: "ignored"})
	a.startProcess = func(cmd *exec.Cmd) error {
		cmd.Process = &os.Process{Pid: 999999}
		cancel()
		return nil
	}
	a.waitProcess = func(*os.Process) (*os.ProcessState, error) { return nil, errors.New("child completed") }
	if err := a.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestChildWaitAndKillErrors(t *testing.T) {
	a, _, _, _, lg := newTestAgent(AgentConfig{})
	a.config.StopDelay = 0
	a.childProcess = &os.Process{Pid: 999999}
	a.childDone = make(chan struct{})
	release := make(chan struct{})
	a.waitProcess = func(*os.Process) (*os.ProcessState, error) {
		<-release
		return nil, nil
	}
	killCalled := make(chan struct{})
	a.killProcess = func(*os.Process) error { close(killCalled); return errors.New("kill") }
	done := make(chan struct{})
	go func() { a.stopChildProcess(); close(done) }()
	select {
	case <-killCalled:
	case <-time.After(time.Second):
		t.Fatal("kill path was not reached")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not complete")
	}
	if lg.ErrorCount() == 0 {
		t.Fatal("expected child errors to be logged")
	}

	a, _, _, _, _ = newTestAgent(AgentConfig{})
	a.childProcess = &os.Process{Pid: 999999}
	a.waitProcess = func(*os.Process) (*os.ProcessState, error) { return nil, nil }
	if err := a.waitForChildProcess(); err == nil {
		t.Fatal("expected missing process state error")
	}
}

func TestValidateVersionFileFstatErrors(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "version")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := ValidateVersionFile(f); err == nil {
		t.Fatal("expected closed file error")
	}
}
