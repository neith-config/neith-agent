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

	t.Run("missing separator after flags", func(t *testing.T) {
		_, err := ParseCommand([]string{"--no-env"})
		if err != ErrMissingSeparator {
			t.Fatalf("expected %v, got %v", ErrMissingSeparator, err)
		}
	})

	t.Run("empty args", func(t *testing.T) {
		_, err := ParseCommand(nil)
		if err != ErrMissingSeparator {
			t.Fatalf("expected %v, got %v", ErrMissingSeparator, err)
		}
	})

	t.Run("json output without separator", func(t *testing.T) {
		_, err := ParseCommand([]string{"--json-output", "/tmp/env.json"})
		if err != ErrMissingSeparator {
			t.Fatalf("expected %v, got %v", ErrMissingSeparator, err)
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
}
