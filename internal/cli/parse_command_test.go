package cli

import "testing"

func TestParseCommand(t *testing.T) {
	t.Parallel()

	t.Run("missing separator", func(t *testing.T) {
		_, err := ParseCommand([]string{"getenv"})
		if err != ErrMissingSeparator {
			t.Fatalf("expected %v, got %v", ErrMissingSeparator, err)
		}
	})

	t.Run("json output followed by flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--json-output", "/tmp/env.json", "--no-env", "--", "env"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.NoEnv {
			t.Fatalf("expected no-env flag")
		}
		if opts.JsonOutputPath != "/tmp/env.json" {
			t.Fatalf("unexpected json output path: %q", opts.JsonOutputPath)
		}
	})

	t.Run("daemon mode after flags", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--no-env"})
		if err != nil || opts.Command != "" {
			t.Fatalf("expected daemon-only options, got %#v, %v", opts, err)
		}
	})

	t.Run("empty args selects daemon mode", func(t *testing.T) {
		opts, err := ParseCommand(nil)
		if err != nil || opts.Command != "" {
			t.Fatalf("expected daemon-only options, got %#v, %v", opts, err)
		}
	})

	t.Run("json output in daemon mode", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--json-output", "/tmp/env.json"})
		if err != nil || opts.JsonOutputPath != "/tmp/env.json" {
			t.Fatalf("expected daemon-only JSON options, got %#v, %v", opts, err)
		}
	})

	t.Run("missing command", func(t *testing.T) {
		_, err := ParseCommand([]string{"--"})
		if err != ErrMissingCommand {
			t.Fatalf("expected %v, got %v", ErrMissingCommand, err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--", "getenv", "A"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.Command != "getenv" {
			t.Fatalf("expected getenv, got %q", opts.Command)
		}
		if len(opts.CommandArgs) != 1 || opts.CommandArgs[0] != "A" {
			t.Fatalf("unexpected args: %#v", opts.CommandArgs)
		}
	})

	t.Run("uses first separator", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--", "getenv", "--", "A"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.Command != "getenv" {
			t.Fatalf("expected getenv, got %q", opts.Command)
		}
		if len(opts.CommandArgs) != 2 || opts.CommandArgs[0] != "--" || opts.CommandArgs[1] != "A" {
			t.Fatalf("unexpected args: %#v", opts.CommandArgs)
		}
	})

	t.Run("parses flags", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--no-env", "--json-output", "/tmp/env.json", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.NoEnv {
			t.Fatalf("expected no-env flag: %#v", opts)
		}
		if opts.NoEnvFile {
			t.Fatalf("expected env file writing to stay enabled: %#v", opts)
		}
		if opts.JsonOutputPath != "/tmp/env.json" {
			t.Fatalf("expected json output path, got %q", opts.JsonOutputPath)
		}
	})

	t.Run("unknown flag", func(t *testing.T) {
		_, err := ParseCommand([]string{"--bogus", "--", "getenv"})
		if err != ErrUnknownFlag {
			t.Fatalf("expected unknown flag err, got %v", err)
		}
	})

	t.Run("missing json output value", func(t *testing.T) {
		_, err := ParseCommand([]string{"--json-output", "--", "getenv"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("missing json output value at end", func(t *testing.T) {
		_, err := ParseCommand([]string{"--json-output"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("no env file flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--no-json-output", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.NoEnvFile {
			t.Fatalf("expected no-json-output flag: %#v", opts)
		}
	})

	t.Run("conflicting flags", func(t *testing.T) {
		_, err := ParseCommand([]string{"--no-json-output", "--json-output", "/tmp/env.json", "--", "getenv"})
		if err != ErrConflictingFlags {
			t.Fatalf("expected conflicting flags err, got %v", err)
		}
	})

	t.Run("conflicting flags reverse order", func(t *testing.T) {
		_, err := ParseCommand([]string{"--json-output", "/tmp/env.json", "--no-json-output", "--", "getenv"})
		if err != ErrConflictingFlags {
			t.Fatalf("expected conflicting flags err, got %v", err)
		}
	})

	t.Run("isFlag", func(t *testing.T) {
		t.Parallel()
		if !isFlag("--foo") {
			t.Fatalf("expected --foo to be flag")
		}
		if isFlag("--") {
			t.Fatalf("expected -- to be separator, not flag")
		}
		if isFlag("foo") {
			t.Fatalf("expected foo not to be flag")
		}
	})

	t.Run("version flag without separator", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--version"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Version {
			t.Fatalf("expected version flag to be set: %#v", opts)
		}
	})

	t.Run("version flag with separator ignored", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--version", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Version {
			t.Fatalf("expected version flag to be set: %#v", opts)
		}
	})

	t.Run("help subcommand", func(t *testing.T) {
		opts, err := ParseCommand([]string{"help"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("help subcommand with args ignored", func(t *testing.T) {
		opts, err := ParseCommand([]string{"help", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("--help flag without separator", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--help"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("--help flag with separator ignored", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--help", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("-h flag without separator", func(t *testing.T) {
		opts, err := ParseCommand([]string{"-h"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("-h flag with separator ignored", func(t *testing.T) {
		opts, err := ParseCommand([]string{"-h", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Help {
			t.Fatalf("expected help flag to be set: %#v", opts)
		}
	})

	t.Run("hub-url flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--hub-url", "http://hub:8080", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.HubURL != "http://hub:8080" {
			t.Fatalf("expected hub URL, got %q", opts.HubURL)
		}
	})

	t.Run("hub-url flag missing value", func(t *testing.T) {
		_, err := ParseCommand([]string{"--hub-url", "--", "getenv"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("hub-url flag missing value at end", func(t *testing.T) {
		_, err := ParseCommand([]string{"--hub-url"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("token flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--token", "secret-token", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.Token != "secret-token" {
			t.Fatalf("expected token, got %q", opts.Token)
		}
	})

	t.Run("token flag missing value", func(t *testing.T) {
		_, err := ParseCommand([]string{"--token", "--", "getenv"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("token flag missing value at end", func(t *testing.T) {
		_, err := ParseCommand([]string{"--token"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("poll-interval flag valid", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--poll-interval", "10", "--", "getenv"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.PollInterval != 10 {
			t.Fatalf("expected poll interval 10, got %d", opts.PollInterval)
		}
	})

	t.Run("poll-interval flag invalid number", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval", "invalid", "--", "getenv"})
		if err == nil || err.Error() != "invalid poll interval: must be a number" {
			t.Fatalf("expected invalid number err, got %v", err)
		}
	})

	t.Run("poll-interval flag too small", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval", "0", "--", "getenv"})
		if err == nil || err.Error() != "invalid poll interval: must be >= 1 second" {
			t.Fatalf("expected invalid interval err, got %v", err)
		}
	})

	t.Run("poll-interval flag negative", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval", "-5", "--", "getenv"})
		if err == nil || err.Error() != "invalid poll interval: must be >= 1 second" {
			t.Fatalf("expected invalid interval err, got %v", err)
		}
	})

	t.Run("poll-interval flag missing value", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval", "--", "getenv"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected missing flag value err, got %v", err)
		}
	})

	t.Run("all new flags together", func(t *testing.T) {
		opts, err := ParseCommand([]string{
			"--hub-url", "http://example.com:9090",
			"--token", "my-secret-token",
			"--poll-interval", "15",
			"--", "getenv",
		})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if opts.HubURL != "http://example.com:9090" {
			t.Fatalf("expected hub URL, got %q", opts.HubURL)
		}
		if opts.Token != "my-secret-token" {
			t.Fatalf("expected token, got %q", opts.Token)
		}
		if opts.PollInterval != 15 {
			t.Fatalf("expected poll interval 15, got %d", opts.PollInterval)
		}
	})

	t.Run("daemon mode without command", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--hub-url", "http://hub:8080"})
		if err != nil {
			t.Fatalf("expected daemon mode to be accepted, got %v", err)
		}
		if opts.HubURL != "http://hub:8080" || opts.Command != "" {
			t.Fatalf("unexpected daemon options: %#v", opts)
		}
	})

	t.Run("token flag value is separator", func(t *testing.T) {
		_, err := ParseCommand([]string{"--token", "--"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected ErrMissingFlagValue, got %v", err)
		}
	})

	t.Run("poll-interval flag value is separator", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval", "--"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected ErrMissingFlagValue, got %v", err)
		}
	})

	t.Run("poll-interval flag missing value at end", func(t *testing.T) {
		_, err := ParseCommand([]string{"--poll-interval"})
		if err != ErrMissingFlagValue {
			t.Fatalf("expected ErrMissingFlagValue, got %v", err)
		}
	})

	t.Run("verbose flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"--verbose", "--", "cmd"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Verbose {
			t.Fatal("expected Verbose to be true")
		}
	})

	t.Run("-v flag", func(t *testing.T) {
		opts, err := ParseCommand([]string{"-v", "--", "cmd"})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !opts.Verbose {
			t.Fatal("expected Verbose to be true via -v")
		}
	})
}
