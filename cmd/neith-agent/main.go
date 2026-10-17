package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/neith-config/neith-agent/internal/agent"
	"github.com/neith-config/neith-agent/internal/cli"
	"github.com/neith-config/neith-agent/internal/config"
	"github.com/neith-config/neith-agent/internal/logger"
	"github.com/neith-config/neith-agent/internal/runner"
)

var Version = "dev"

const DefaultConfigFetchTimeout = 10 * time.Second

const helpText = `Usage: neith-agent [FLAGS] [-- COMMAND [ARGS...]]

Run as unified agent that fetches config and wraps child process.

The agent runs as a daemon, continuously fetching configuration from the hub
and optionally running a child process with the injected configuration.

FLAGS:
  --verbose               Enable debug logging output to stderr
  --json-output PATH      Write configuration to JSON file
                          (default: $XDG_CONFIG_HOME/neith-agent/env.json)
  --no-env                Skip injecting configuration as environment variables
  --no-json-output        Skip writing JSON configuration file
  --hub-url URL           Hub server URL (default: http://localhost:8080)
  --token TOKEN           Authentication token for hub
  --poll-interval SECONDS Polling interval in seconds (default: 5)
  --version               Print version and exit
  --help, -h              Show this help message

Explicit Hub/token flags override NEITH_HUB_URL/NEITH_TOKEN environment values.

MODES:
  neith-agent [FLAGS]                    # Daemon mode only
  neith-agent [FLAGS] -- COMMAND [ARGS...] # Daemon + child process

EXAMPLES:
  neith-agent                              # Daemon mode only
  neith-agent --hub-url http://hub:8080   # Daemon with custom hub
  neith-agent -- mycommand                # Run daemon with child process
  neith-agent --verbose -- mycommand      # Daemon + child with debug logging
  neith-agent --json-output /tmp/config.json -- mycommand

EXIT CODES:
  0                       Success or help/version displayed
  1                       Configuration/setup error
  2                       Command-line parsing error
  126                     Permission denied
  127                     Command not found
  N                       Target process exit code`

var runRunner = runner.Run
var osExit = os.Exit
var osOpenFile = os.OpenFile
var agentNewVersionStore = agent.NewVersionStore
var versionFileChmod = func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) }

func main() {
	osExit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	return runWithUnifiedMode(args, stderr, runUnifiedMode)
}

func runWithUnifiedMode(args []string, stderr io.Writer, runMode func(cli.Options, string, string, string, logger.Logger, agent.SnapshotFetcher, agent.SSESubscriber, agent.VersionStore) int) int {
	opts, err := cli.ParseCommand(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err.Error())
		return 2
	}

	if opts.Help {
		fmt.Println(helpText)
		return 0
	}

	if opts.Version {
		fmt.Println("neith-agent version:", Version)
		return 0
	}

	lg := logger.NewLogger(opts.Verbose)

	hubURL := resolveHubURL(opts.HubURL)
	token := resolveToken(opts.Token)
	if token == "" {
		_, _ = fmt.Fprintln(stderr, "NEITH_TOKEN is required")
		return 1
	}

	serviceName := getEnvOrDefault("NEITH_SERVICE", "default")
	if err := agent.ValidateServiceName(serviceName); err != nil {
		lg.Error("Invalid service name", err)
		return 1
	}
	versionFilePath := filepath.Join(agent.ShmDir(), fmt.Sprintf(agent.VersionFileName, serviceName))

	vFile, err := osOpenFile(versionFilePath, os.O_RDWR|os.O_CREATE|agent.VersionFileOpenFlags(), 0o600)
	if err != nil {
		lg.Error("Failed to open version file", err)
		return 1
	}
	if err := agent.ValidateVersionFile(vFile); err != nil {
		lg.Error("Invalid version file", err)
		_ = vFile.Close()
		return 1
	}
	if err := versionFileChmod(vFile, 0o600); err != nil {
		lg.Error("Failed to restrict version file permissions", err)
		_ = vFile.Close()
		return 1
	}

	verStore, err := agentNewVersionStore(vFile)
	if err != nil {
		lg.Error("Failed to initialize version store", err)
		_ = vFile.Close()
		return 1
	}

	pollClient := config.NewHubHTTPClient(hubURL, token, newPollingHTTPClient())
	sseClient := config.NewHubSSEClient(hubURL, token, newStreamingHTTPClient(), lg, config.DefaultSleep, rand.Float64)

	return runMode(opts, hubURL, token, serviceName, lg, pollClient, sseClient, verStore)
}

func runUnifiedMode(
	opts cli.Options,
	hubURL, token, serviceName string,
	lg logger.Logger,
	pollClient agent.SnapshotFetcher,
	sseClient agent.SSESubscriber,
	verStore agent.VersionStore,
) int {
	return runUnifiedModeWithRunner(opts, hubURL, token, serviceName, lg, pollClient, sseClient, verStore,
		func(a *agent.Agent, ctx context.Context) error { return a.Run(ctx) })
}

func runUnifiedModeWithRunner(
	opts cli.Options,
	hubURL, token, serviceName string,
	lg logger.Logger,
	pollClient agent.SnapshotFetcher,
	sseClient agent.SSESubscriber,
	verStore agent.VersionStore,
	runAgent func(*agent.Agent, context.Context) error,
) int {
	agentCfg := agent.AgentConfig{
		HubURL:         hubURL,
		Token:          token,
		ServiceName:    serviceName,
		PollInterval:   opts.PollInterval,
		Verbose:        opts.Verbose,
		Command:        opts.Command,
		Args:           opts.CommandArgs,
		InjectEnv:      !opts.NoEnv,
		WriteJSONFile:  !opts.NoEnvFile,
		JSONOutputPath: opts.JsonOutputPath,
	}

	agnt := agent.NewAgent(agentCfg, pollClient, sseClient, verStore, lg)

	if err := runAgent(agnt, context.Background()); err != nil {
		var childExit *agent.ChildExitError
		if errors.As(err, &childExit) {
			lg.Error("Child process exited", err)
			return childExit.Code
		}
		lg.Error("Agent failed", err)
		return 1
	}

	return 0
}

func newPollingHTTPClient() *http.Client {
	return &http.Client{Timeout: DefaultConfigFetchTimeout}
}

func newStreamingHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: DefaultConfigFetchTimeout}).DialContext
	transport.TLSHandshakeTimeout = DefaultConfigFetchTimeout
	transport.ResponseHeaderTimeout = DefaultConfigFetchTimeout
	transport.ExpectContinueTimeout = DefaultConfigFetchTimeout
	return &http.Client{Transport: transport}
}

func resolveHubURL(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if value := os.Getenv("NEITH_HUB_URL"); value != "" {
		return value
	}
	return agent.DefaultHubURL
}

func resolveToken(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("NEITH_TOKEN")
}

func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}
