package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
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
var runUnifiedModeFn = runUnifiedMode
var agentRunFn = func(a *agent.Agent) error { return a.Run(context.Background()) }

func main() {
	osExit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
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

	hubURL := getEnvOrDefault("NEITH_HUB_URL", opts.HubURL)
	token := getEnvOrDefault("NEITH_TOKEN", opts.Token)
	if hubURL == "" || token == "" {
		_, _ = fmt.Fprintln(stderr, "NEITH_HUB_URL and NEITH_TOKEN are required")
		return 1
	}

	serviceName := getEnvOrDefault("NEITH_SERVICE", "default")
	versionFilePath := filepath.Join(agent.ShmDir(), fmt.Sprintf(agent.VersionFileName, serviceName))

	vFile, err := osOpenFile(versionFilePath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		lg.Error("Failed to open version file", err)
		return 1
	}

	verStore, err := agentNewVersionStore(vFile)
	if err != nil {
		lg.Error("Failed to initialize version store", err)
		_ = vFile.Close()
		return 1
	}

	httpClient := &http.Client{Timeout: DefaultConfigFetchTimeout}
	pollClient := config.NewHubHTTPClient(hubURL, token, httpClient)
	sseClient := config.NewHubSSEClient(hubURL, token, httpClient, lg, config.DefaultSleep, rand.Float64)

	return runUnifiedModeFn(opts, hubURL, token, serviceName, lg, pollClient, sseClient, verStore)
}

func runUnifiedMode(
	opts cli.Options,
	hubURL, token, serviceName string,
	lg logger.Logger,
	pollClient runner.EnvFetcher,
	sseClient agent.SSESubscriber,
	verStore agent.VersionStore,
) int {
	agentCfg := agent.AgentConfig{
		HubURL:       hubURL,
		Token:        token,
		ServiceName:  serviceName,
		PollInterval: opts.PollInterval,
		Verbose:      opts.Verbose,
		Command:      opts.Command,
		Args:         opts.CommandArgs,
		InjectEnv:    !opts.NoEnv,
	}

	agnt := agent.NewAgent(agentCfg, pollClient, sseClient, verStore, lg)

	if err := agentRunFn(agnt); err != nil {
		lg.Error("Agent failed", err)
		return 1
	}

	return 0
}

func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}
