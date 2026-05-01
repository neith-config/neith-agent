package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/neth-config/neth-agent/internal/logger"
)

type EnvFetcher interface {
	Fetch(ctx context.Context) (map[string]string, error)
}

type RunConfig struct {
	Command        string
	Args           []string
	InjectEnv      bool
	WriteEnvFile   bool
	JsonOutputPath string
	Verbose        bool
}

var commandContext = exec.CommandContext
var forwardSignalsFunc = forwardSignals
var notifyProcessStartedFunc = notifyProcessStarted
var startCmd = func(cmd *exec.Cmd) error { return cmd.Start() }
var waitCmd = func(cmd *exec.Cmd) error { return cmd.Wait() }
var resolveEnvFilePath = defaultEnvFilePath
var writeEnvFileFunc = writeEnvFile
var validateEnvFilePath = ensureWritableEnvFile

func Run(ctx context.Context, fetcher EnvFetcher, lg logger.Logger, cfg RunConfig) int {
	lg.Info("Fetching configuration")
	env, err := fetcher.Fetch(ctx)
	if err != nil {
		lg.Error("Failed to fetch configuration", err)
		return 1
	}

	lg.Debug(fmt.Sprintf("Configuration fetched with %d entries", len(env)))

	if cfg.WriteEnvFile {
		path := cfg.JsonOutputPath
		if path == "" {
			var err error
			path, err = resolveEnvFilePath()
			if err != nil {
				lg.Error("Failed to resolve env file path", err)
				return 1
			}
		}
		if err := validateEnvFilePath(path); err != nil {
			lg.Error("Invalid env file path", err)
			return 1
		}
		if err := writeEnvFileFunc(path, env); err != nil {
			lg.Error("Failed to write env file", err)
			return 1
		}
		lg.Info(fmt.Sprintf("Writing JSON output to %s", path))
	}

	cmd := commandContext(ctx, cfg.Command, cfg.Args...)
	if cfg.InjectEnv {
		baseEnv := envToMap(os.Environ())
		cmd.Env = MergeEnv(baseEnv, env, lg)
		lg.Debug("Environment variables injected")
	}
	configureCmdStdio(cmd)

	cleanupSignals := forwardSignalsFunc(cmd)
	defer cleanupSignals()

	// Check if context was already cancelled or timed out before starting the command.
	// The timeout is for config fetch phase only; command execution is not limited by this timeout.
	if err := ctx.Err(); err != nil {
		lg.Error("Context cancelled or timed out before command execution", err)
		return 1
	}

	lg.Info(fmt.Sprintf("Starting process: %s", cfg.Command))
	if err := startCmd(cmd); err != nil {
		lg.Error("Failed to start process", err)
		return exitCodeFromStartError(err)
	}

	notifyProcessStartedFunc(cmd)

	if err := waitCmd(cmd); err != nil {
		return exitCodeFromWaitError(err)
	}

	return 0
}

func configureCmdStdio(cmd *exec.Cmd) {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func exitCodeFromWaitError(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func exitCodeFromStartError(err error) int {
	var execErr *exec.Error
	if errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
		return 127
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		// Best-effort mapping to common shell conventions.
		if errors.Is(pathErr.Err, os.ErrPermission) {
			return 126
		}
	}

	return 1
}
