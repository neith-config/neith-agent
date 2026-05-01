package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

type stubFetcher struct {
	env map[string]string
	err error
}

type noOpLogger struct{}

func (l *noOpLogger) Debug(msg string)              {}
func (l *noOpLogger) Info(msg string)               {}
func (l *noOpLogger) Warning(msg string)            {}
func (l *noOpLogger) Error(msg string, err error)   {}

func TestRun_ResolveEnvFilePathError(t *testing.T) {
	oldResolve := resolveEnvFilePath
	oldValidate := validateEnvFilePath
	oldWrite := writeEnvFileFunc
	defer func() {
		resolveEnvFilePath = oldResolve
		validateEnvFilePath = oldValidate
		writeEnvFileFunc = oldWrite
	}()

	resolveErr := errors.New("boom")
	resolveEnvFilePath = func() (string, error) {
		return "", resolveErr
	}
	validateEnvFilePath = func(string) error {
		t.Fatalf("validateEnvFilePath should not be called when resolveEnvFilePath fails")
		return nil
	}
	writeEnvFileFunc = func(string, map[string]string) error {
		t.Fatalf("writeEnvFileFunc should not be called when resolveEnvFilePath fails")
		return nil
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command:      "ignored",
		WriteEnvFile: true,
	})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRun_WritesEnvFile(t *testing.T) {
	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	oldStart := startCmd
	oldWait := waitCmd
	oldWrite := writeEnvFileFunc
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
		startCmd = oldStart
		waitCmd = oldWait
		writeEnvFileFunc = oldWrite
	}()

	commandContext = func(context.Context, string, ...string) *exec.Cmd { return &exec.Cmd{} }
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(*exec.Cmd) error { return nil }
	waitCmd = func(*exec.Cmd) error { return nil }

	var recordedPath string
	var recordedEnv map[string]string
	writeEnvFileFunc = func(path string, env map[string]string) error {
		recordedPath = path
		recordedEnv = env
		return nil
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{"A": "1"}}, &noOpLogger{}, RunConfig{
		Command:        "ignored",
		WriteEnvFile:   true,
		JsonOutputPath: "/tmp/example.json",
	})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if recordedPath != "/tmp/example.json" {
		t.Fatalf("unexpected path: %q", recordedPath)
	}
	if recordedEnv["A"] != "1" {
		t.Fatalf("unexpected env: %#v", recordedEnv)
	}
}

func TestRun_ValidateEnvFilePathError(t *testing.T) {
	oldResolve := resolveEnvFilePath
	oldValidate := validateEnvFilePath
	defer func() {
		resolveEnvFilePath = oldResolve
		validateEnvFilePath = oldValidate
	}()

	resolveEnvFilePath = func() (string, error) {
		return "/tmp/example.json", nil
	}
	validateEnvFilePath = func(string) error { return errors.New("denied") }

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command:      "ignored",
		WriteEnvFile: true,
	})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRun_WriteEnvFileDefaultPathError(t *testing.T) {
	oldResolve := resolveEnvFilePath
	defer func() { resolveEnvFilePath = oldResolve }()
	resolveEnvFilePath = func() (string, error) {
		return "", errors.New("boom")
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command:      "ignored",
		WriteEnvFile: true,
	})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRun_WriteEnvFileWriteError(t *testing.T) {
	oldResolve := resolveEnvFilePath
	oldWrite := writeEnvFileFunc
	oldValidate := validateEnvFilePath
	defer func() {
		resolveEnvFilePath = oldResolve
		writeEnvFileFunc = oldWrite
		validateEnvFilePath = oldValidate
	}()

	resolveEnvFilePath = func() (string, error) {
		return "/tmp/example.json", nil
	}
	validateEnvFilePath = func(string) error { return nil }
	writeEnvFileFunc = func(string, map[string]string) error {
		return errors.New("boom")
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command:      "ignored",
		WriteEnvFile: true,
	})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRun_ConfiguresStdioBeforeStart(t *testing.T) {
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

	cmd := &exec.Cmd{}
	commandContext = func(context.Context, string, ...string) *exec.Cmd { return cmd }
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	waitCmd = func(*exec.Cmd) error { t.Fatalf("unexpected wait"); return nil }

	startCmd = func(c *exec.Cmd) error {
		if c.Stdin != os.Stdin || c.Stdout != os.Stdout || c.Stderr != os.Stderr {
			t.Fatalf("stdio not configured")
		}
		return errors.New("boom")
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{Command: "ignored"})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestConfigureCmdStdio(t *testing.T) {
	t.Parallel()

	cmd := &exec.Cmd{}
	configureCmdStdio(cmd)

	if cmd.Stdin != os.Stdin {
		t.Fatalf("expected stdin to be set")
	}
	if cmd.Stdout != os.Stdout {
		t.Fatalf("expected stdout to be set")
	}
	if cmd.Stderr != os.Stderr {
		t.Fatalf("expected stderr to be set")
	}
}

func (s stubFetcher) Fetch(context.Context) (map[string]string, error) {
	return s.env, s.err
}

func TestRun_FetchError(t *testing.T) {
	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
	}()

	commandContext = func(context.Context, string, ...string) *exec.Cmd {
		panic("commandContext should not be called")
	}
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }

	code := Run(context.Background(), stubFetcher{err: errors.New("x")}, &noOpLogger{}, RunConfig{Command: "ignored"})
	if code != 1 {
		t.Fatalf("expected 1, got %d", code)
	}
}

func TestRun_StartNotFound(t *testing.T) {
	oldForward := forwardSignalsFunc
	defer func() { forwardSignalsFunc = oldForward }()

	cleanupCalled := false
	forwardSignalsFunc = func(*exec.Cmd) func() {
		return func() { cleanupCalled = true }
	}

	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{Command: "__definitely_not_a_real_command__"})
	if code != 127 {
		t.Fatalf("expected 127, got %d", code)
	}
	if !cleanupCalled {
		t.Fatalf("expected cleanup to be called")
	}
}

func TestRun_ExitCodePropagation(t *testing.T) {
	t.Parallel()

	cmd, args := nonZeroExitCommand(7)
	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{Command: cmd, Args: args})
	if code != 7 {
		t.Fatalf("expected 7, got %d", code)
	}
}

func TestRun_Success(t *testing.T) {
	t.Parallel()

	cmd, args := zeroExitCommand()
	code := Run(context.Background(), stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{Command: cmd, Args: args})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
}

func TestRun_InjectsEnv(t *testing.T) {
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

	cmd := &exec.Cmd{}
	commandContext = func(context.Context, string, ...string) *exec.Cmd { return cmd }
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCalled := false
	startCmd = func(c *exec.Cmd) error {
		startCalled = true
		found := false
		for _, kv := range c.Env {
			if kv == "FOO=BAR" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected injected env, got %#v", c.Env)
		}
		return nil
	}
	waitCmd = func(*exec.Cmd) error { return nil }

	code := Run(context.Background(), stubFetcher{env: map[string]string{"FOO": "BAR"}}, &noOpLogger{}, RunConfig{
		Command:   "ignored",
		InjectEnv: true,
	})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !startCalled {
		t.Fatalf("start not called")
	}
}

func TestRun_ContextTimeoutBeforeCommandStart(t *testing.T) {
	// Test that if context is already cancelled/timed out before command start,
	// the runner returns 1 without attempting to start the command.
	oldCommandContext := commandContext
	oldForward := forwardSignalsFunc
	oldStart := startCmd
	defer func() {
		commandContext = oldCommandContext
		forwardSignalsFunc = oldForward
		startCmd = oldStart
	}()

	commandCalled := false
	commandContext = func(context.Context, string, ...string) *exec.Cmd { return &exec.Cmd{} }
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(*exec.Cmd) error {
		commandCalled = true
		return nil
	}

	// Create a context that's already timed out
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond) // Ensure timeout has occurred

	code := Run(ctx, stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command: "ignored",
	})

	if code != 1 {
		t.Fatalf("expected 1 for timed out context, got %d", code)
	}
	if commandCalled {
		t.Fatalf("command should not be started when context is already cancelled")
	}
}

func TestRun_ContextStillValidBeforeCommandStart(t *testing.T) {
	// Test that if context is still valid before command start,
	// the command executes normally (timeout only applies to config fetch phase).
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

	commandCalled := false
	commandContext = func(context.Context, string, ...string) *exec.Cmd { return &exec.Cmd{} }
	forwardSignalsFunc = func(*exec.Cmd) func() { return func() {} }
	startCmd = func(*exec.Cmd) error {
		commandCalled = true
		return nil
	}
	waitCmd = func(*exec.Cmd) error { return nil }

	// Create a context with a long timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	code := Run(ctx, stubFetcher{env: map[string]string{}}, &noOpLogger{}, RunConfig{
		Command: "ignored",
	})

	if code != 0 {
		t.Fatalf("expected 0 for valid context, got %d", code)
	}
	if !commandCalled {
		t.Fatalf("command should be started when context is valid")
	}
}

func TestExitCodeFromWaitError(t *testing.T) {
	t.Parallel()

	if exitCodeFromWaitError(errors.New("x")) != 1 {
		t.Fatalf("expected 1")
	}

	cmd, args := nonZeroExitCommand(9)
	e := exec.Command(cmd, args...)
	_ = e.Run()
	if e.ProcessState.ExitCode() != 9 {
		t.Fatalf("expected exit 9")
	}
	if exitCodeFromWaitError(&exec.ExitError{ProcessState: e.ProcessState}) != 9 {
		t.Fatalf("expected 9")
	}
}

func TestExitCodeFromStartError(t *testing.T) {
	t.Parallel()

	e := exec.Command("__definitely_not_a_real_command__")
	err := e.Start()
	if err == nil {
		t.Fatalf("expected err")
	}
	if exitCodeFromStartError(err) != 127 {
		t.Fatalf("expected 127")
	}

	if exitCodeFromStartError(&os.PathError{Err: os.ErrPermission}) != 126 {
		t.Fatalf("expected 126")
	}

	if exitCodeFromStartError(errors.New("x")) != 1 {
		t.Fatalf("expected 1")
	}

	if exitCodeFromStartError(&exec.Error{Name: "x", Err: errors.New("y")}) != 1 {
		t.Fatalf("expected 1")
	}
}

func nonZeroExitCommand(code int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "exit", itoa(code)}
	}

	return "sh", []string{"-c", "exit " + itoa(code)}
}

func zeroExitCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", "exit", "0"}
	}

	return "sh", []string{"-c", "exit 0"}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}

	neg := false
	if v < 0 {
		neg = true
		v = -v
	}

	buf := make([]byte, 0, 12)
	for v > 0 {
		d := byte(v % 10)
		buf = append(buf, '0'+d)
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
