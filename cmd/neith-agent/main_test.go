package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/neith-config/neith-agent/internal/agent"
	"github.com/neith-config/neith-agent/internal/cli"
	"github.com/neith-config/neith-agent/internal/logger"
	"github.com/neith-config/neith-agent/internal/runner"
)

// agentStubForTest creates a stopped agent immediately.
type agentRunErr struct{ err error }

func (a *agentRunErr) run(_ *agent.Agent) error { return a.err }

// fakeFetcher implements runner.EnvFetcher for tests.
type fakeFetcher struct{ config map[string]string }

func (f *fakeFetcher) Fetch(_ context.Context) (map[string]string, error) {
	return f.config, nil
}

func (f *fakeFetcher) FetchSnapshot(_ context.Context) (agent.Snapshot, error) {
	return agent.Snapshot{Config: f.config}, nil
}

// fakeSSE implements agent.SSESubscriber for tests.
type fakeSSE struct{}

func (f *fakeSSE) Stream(ctx context.Context, _ func(map[string]string)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeSSE) StreamSnapshots(ctx context.Context, _ func(agent.Snapshot)) error {
	<-ctx.Done()
	return ctx.Err()
}

// fakeVersionStore implements agent.VersionStore for tests.
type fakeVersionStore struct{}

func (f *fakeVersionStore) Current() int64 { return 0 }
func (f *fakeVersionStore) Store(_ int64)  {}
func (f *fakeVersionStore) Close() error   { return nil }

func TestRun(t *testing.T) {
	oldRunRunner := runRunner
	defer func() { runRunner = oldRunRunner }()

	t.Run("version flag", func(t *testing.T) {
		called := false
		runRunner = func(_ context.Context, _ runner.EnvFetcher, _ logger.Logger, _ runner.RunConfig) int {
			called = true
			return 0
		}

		var stderr bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		code := run([]string{"--version"}, &stderr)

		w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		buf.ReadFrom(r)

		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
		if called {
			t.Fatal("did not expect runner to be called")
		}
		if !strings.Contains(buf.String(), "neith-agent version:") {
			t.Fatalf("expected version output, got %q", buf.String())
		}
	})

	t.Run("help flag", func(t *testing.T) {
		var stderr bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		code := run([]string{"--help"}, &stderr)

		w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		buf.ReadFrom(r)

		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
		if !strings.Contains(buf.String(), "Usage: neith-agent") {
			t.Fatalf("expected help output, got %q", buf.String())
		}
	})

	t.Run("-h flag", func(t *testing.T) {
		var stderr bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		code := run([]string{"-h"}, &stderr)

		w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		buf.ReadFrom(r)

		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
		if !strings.Contains(buf.String(), "Usage: neith-agent") {
			t.Fatalf("expected help output, got %q", buf.String())
		}
	})

	t.Run("help subcommand", func(t *testing.T) {
		var stderr bytes.Buffer
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		code := run([]string{"help"}, &stderr)

		w.Close()
		os.Stdout = oldStdout
		var buf bytes.Buffer
		buf.ReadFrom(r)

		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
		if !strings.Contains(buf.String(), "Usage: neith-agent") {
			t.Fatalf("expected help output, got %q", buf.String())
		}
	})

	t.Run("invalid args", func(t *testing.T) {
		var stderr bytes.Buffer
		code := run([]string{"getenv"}, &stderr)
		if code != 2 {
			t.Fatalf("expected 2, got %d", code)
		}
		if stderr.Len() == 0 {
			t.Fatal("expected stderr output")
		}
	})

	t.Run("missing hub url", func(t *testing.T) {
		os.Unsetenv("NEITH_HUB_URL")
		os.Unsetenv("NEITH_TOKEN")
		var stderr bytes.Buffer
		code := run([]string{"--token", "tok", "--", "echo", "hi"}, &stderr)
		if code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "NEITH_HUB_URL") {
			t.Fatalf("expected error mentioning NEITH_HUB_URL, got %q", stderr.String())
		}
	})

	t.Run("missing token", func(t *testing.T) {
		os.Unsetenv("NEITH_HUB_URL")
		os.Unsetenv("NEITH_TOKEN")
		var stderr bytes.Buffer
		code := run([]string{"--hub-url", "http://hub", "--", "echo", "hi"}, &stderr)
		if code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
		if !strings.Contains(stderr.String(), "NEITH_HUB_URL") {
			t.Fatalf("expected error about missing credentials, got %q", stderr.String())
		}
	})

	t.Run("version file open error", func(t *testing.T) {
		oldOpen := osOpenFile
		defer func() { osOpenFile = oldOpen }()
		osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return nil, errors.New("open error")
		}

		t.Setenv("NEITH_HUB_URL", "http://hub")
		t.Setenv("NEITH_TOKEN", "tok")

		var stderr bytes.Buffer
		code := run([]string{"--", "echo", "hi"}, &stderr)
		if code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
	})

	t.Run("version store init error", func(t *testing.T) {
		oldNew := agentNewVersionStore
		defer func() { agentNewVersionStore = oldNew }()
		agentNewVersionStore = func(_ *os.File) (agent.VersionStore, error) {
			return nil, errors.New("mmap error")
		}

		oldOpen := osOpenFile
		defer func() { osOpenFile = oldOpen }()
		osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return os.CreateTemp(t.TempDir(), "v")
		}

		t.Setenv("NEITH_HUB_URL", "http://hub")
		t.Setenv("NEITH_TOKEN", "tok")

		var stderr bytes.Buffer
		code := run([]string{"--", "echo", "hi"}, &stderr)
		if code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
	})

	t.Run("successful setup reaches runUnifiedModeFn", func(t *testing.T) {
		oldOpen := osOpenFile
		defer func() { osOpenFile = oldOpen }()
		osOpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			return os.CreateTemp(t.TempDir(), "v")
		}

		oldFn := runUnifiedModeFn
		defer func() { runUnifiedModeFn = oldFn }()
		runUnifiedModeFn = func(_ cli.Options, _, _, _ string, _ logger.Logger, _ agent.SnapshotFetcher, _ agent.SSESubscriber, _ agent.VersionStore) int {
			return 0
		}

		t.Setenv("NEITH_HUB_URL", "http://hub")
		t.Setenv("NEITH_TOKEN", "tok")

		var stderr bytes.Buffer
		code := run([]string{"--", "echo", "hi"}, &stderr)
		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
	})
}

func TestRunRejectsPathLikeServiceIdentity(t *testing.T) {
	for _, serviceName := range []string{"../escape", "nested/name", "."} {
		t.Run(serviceName, func(t *testing.T) {
			oldOpen := osOpenFile
			defer func() { osOpenFile = oldOpen }()

			opened := false
			osOpenFile = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				opened = true
				return nil, errors.New("version file must not be opened")
			}
			t.Setenv("NEITH_HUB_URL", "http://hub")
			t.Setenv("NEITH_TOKEN", "tok")
			t.Setenv("NEITH_SERVICE", serviceName)

			var stderr bytes.Buffer
			if code := run([]string{"--", "echo", "hi"}, &stderr); code != 1 {
				t.Fatalf("expected setup failure, got %d", code)
			}
			if opened {
				t.Fatal("opened version file for invalid service identity")
			}
		})
	}
}

func TestRunUsesPrivateVersionFileMode(t *testing.T) {
	oldOpen := osOpenFile
	defer func() { osOpenFile = oldOpen }()

	var gotPerm os.FileMode
	osOpenFile = func(_ string, _ int, perm os.FileMode) (*os.File, error) {
		gotPerm = perm
		return nil, errors.New("stop after mode capture")
	}
	t.Setenv("NEITH_HUB_URL", "http://hub")
	t.Setenv("NEITH_TOKEN", "tok")

	var stderr bytes.Buffer
	if code := run([]string{"--", "echo", "hi"}, &stderr); code != 1 {
		t.Fatalf("expected setup failure, got %d", code)
	}
	if gotPerm != 0o600 {
		t.Fatalf("version file mode = %04o, want 0600", gotPerm)
	}
}

func TestMain(t *testing.T) {
	oldArgs := os.Args
	oldExit := osExit
	oldRunRunner := runRunner
	defer func() {
		os.Args = oldArgs
		osExit = oldExit
		runRunner = oldRunRunner
	}()

	exitCode := -1
	osExit = func(code int) { exitCode = code }

	t.Run("fails if argv begins with separator", func(t *testing.T) {
		exitCode = -1
		os.Args = []string{"--", "getenv"}
		main()
		if exitCode != 2 {
			t.Fatalf("expected exit 2, got %d", exitCode)
		}
	})
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		result := getEnvOrDefault("NON_EXISTENT_VAR", "default_value")
		if result != "default_value" {
			t.Fatalf("expected default_value, got %s", result)
		}
	})

	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_ENV_VAR", "env_value")
		result := getEnvOrDefault("TEST_ENV_VAR", "default_value")
		if result != "env_value" {
			t.Fatalf("expected env_value, got %s", result)
		}
	})
}

func TestRunUnifiedMode(t *testing.T) {
	t.Run("daemon starts and runs", func(t *testing.T) {
		opts := cli.Options{
			PollInterval: 5,
		}

		done := make(chan int, 1)
		go func() {
			code := runUnifiedMode(opts, "http://hub", "tok", "svc",
				logger.NewLogger(false),
				&fakeFetcher{config: map[string]string{}},
				&fakeSSE{},
				&fakeVersionStore{},
			)
			done <- code
		}()

		time.Sleep(50 * time.Millisecond)

		select {
		case code := <-done:
			if code != 0 {
				t.Fatalf("expected exit 0, got %d", code)
			}
		case <-time.After(500 * time.Millisecond):
			// expected: daemon is running
		}
	})

	t.Run("agent run error returns 1", func(t *testing.T) {
		oldFn := agentRunFn
		defer func() { agentRunFn = oldFn }()
		agentRunFn = func(_ *agent.Agent) error { return errors.New("run error") }

		opts := cli.Options{PollInterval: 5}
		code := runUnifiedMode(opts, "http://hub", "tok", "svc",
			logger.NewLogger(false),
			&fakeFetcher{config: map[string]string{}},
			&fakeSSE{},
			&fakeVersionStore{},
		)
		if code != 1 {
			t.Fatalf("expected 1, got %d", code)
		}
	})

	t.Run("child exit status is returned", func(t *testing.T) {
		oldFn := agentRunFn
		defer func() { agentRunFn = oldFn }()
		agentRunFn = func(_ *agent.Agent) error { return &agent.ChildExitError{Code: 42} }

		opts := cli.Options{PollInterval: 5}
		code := runUnifiedMode(opts, "http://hub", "tok", "svc",
			logger.NewLogger(false),
			&fakeFetcher{config: map[string]string{}},
			&fakeSSE{},
			&fakeVersionStore{},
		)
		if code != 42 {
			t.Fatalf("expected 42, got %d", code)
		}
	})

	t.Run("agent run success returns 0", func(t *testing.T) {
		oldFn := agentRunFn
		defer func() { agentRunFn = oldFn }()
		agentRunFn = func(_ *agent.Agent) error { return nil }

		opts := cli.Options{PollInterval: 5}
		code := runUnifiedMode(opts, "http://hub", "tok", "svc",
			logger.NewLogger(false),
			&fakeFetcher{config: map[string]string{}},
			&fakeSSE{},
			&fakeVersionStore{},
		)
		if code != 0 {
			t.Fatalf("expected 0, got %d", code)
		}
	})
}

func TestRunDaemonModeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	_, err := cli.ParseCommand([]string{"--hub-url", "http://test.com"})
	if err == nil {
		t.Fatal("expected error for daemon mode without separator")
	}
	if err.Error() != "missing -- separator" {
		t.Fatalf("expected separator error, got %v", err)
	}
}
