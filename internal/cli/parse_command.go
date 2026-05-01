package cli

import (
	"errors"
	"strings"
)

type Options struct {
	Command        string
	CommandArgs    []string
	NoEnv          bool
	NoEnvFile      bool
	JsonOutputPath string
	Verbose        bool
	Version        bool
	Help           bool
}

var ErrMissingSeparator = errors.New("missing -- separator")
var ErrMissingCommand = errors.New("missing command after --")
var ErrUnknownFlag = errors.New("unknown flag")
var ErrMissingFlagValue = errors.New("missing value for flag")
var ErrConflictingFlags = errors.New("conflicting flags")

func ParseCommand(args []string) (Options, error) {
	var opts Options
	sep := -1
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			sep = i
			break
		}
		// Check for "help" as a standalone subcommand (first non-flag arg)
		if a == "help" {
			opts.Help = true
			return opts, nil
		}
		if isFlag(a) {
			if err := applyFlag(&opts, a, args, &i); err != nil {
				return Options{}, err
			}
			// If --version or --help was encountered, return early without requiring separator
			if opts.Version || opts.Help {
				return opts, nil
			}
			continue
		}
		return Options{}, ErrMissingSeparator
	}
	if sep == -1 {
		return Options{}, ErrMissingSeparator
	}
	if sep == len(args)-1 {
		return Options{}, ErrMissingCommand
	}

	opts.Command = args[sep+1]
	opts.CommandArgs = args[sep+2:]

	// Check for conflicting flags
	if opts.NoEnvFile && opts.JsonOutputPath != "" {
		return Options{}, ErrConflictingFlags
	}

	return opts, nil
}

func applyFlag(opts *Options, flag string, args []string, idx *int) error {
	switch flag {
	case "--no-env":
		opts.NoEnv = true
		return nil
	case "--no-json-output":
		opts.NoEnvFile = true
		return nil
	case "--json-output":
		if *idx+1 >= len(args) {
			return ErrMissingFlagValue
		}
		if args[*idx+1] == "--" {
			return ErrMissingFlagValue
		}
		*idx = *idx + 1
		opts.JsonOutputPath = args[*idx]
		return nil
	case "--verbose", "-v":
		opts.Verbose = true
		return nil
	case "--version":
		opts.Version = true
		return nil
	case "--help", "-h":
		opts.Help = true
		return nil
	default:
		return ErrUnknownFlag
	}
}

func isFlag(arg string) bool {
	return (strings.HasPrefix(arg, "--") && arg != "--") || arg == "-h" || arg == "-v"
}
