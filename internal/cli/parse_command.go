package cli

import (
	"errors"
	"strconv"
	"strings"
	"time"
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
	HubURL         string
	Token          string
	PollInterval   int
}

var ErrMissingSeparator = errors.New("missing -- separator")
var ErrMissingCommand = errors.New("missing command after --")
var ErrUnknownFlag = errors.New("unknown flag")
var ErrMissingFlagValue = errors.New("missing value for flag")
var ErrConflictingFlags = errors.New("conflicting flags")

const maxPollIntervalSeconds = int64((time.Duration(1<<63-1) / 5 * 4) / time.Second)

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
		if args[*idx+1] == "--" || isFlag(args[*idx+1]) {
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
	case "--hub-url":
		if *idx+1 >= len(args) {
			return ErrMissingFlagValue
		}
		if args[*idx+1] == "--" {
			return ErrMissingFlagValue
		}
		*idx = *idx + 1
		opts.HubURL = args[*idx]
		return nil
	case "--token":
		if *idx+1 >= len(args) {
			return ErrMissingFlagValue
		}
		if args[*idx+1] == "--" {
			return ErrMissingFlagValue
		}
		*idx = *idx + 1
		opts.Token = args[*idx]
		return nil
	case "--poll-interval":
		if *idx+1 >= len(args) {
			return ErrMissingFlagValue
		}
		if args[*idx+1] == "--" {
			return ErrMissingFlagValue
		}
		*idx = *idx + 1
		var err error
		opts.PollInterval, err = parsePollInterval(args[*idx])
		if err != nil {
			return err
		}
		return nil
	default:
		return ErrUnknownFlag
	}
}

func parsePollInterval(s string) (int, error) {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, errors.New("invalid poll interval: must be a number")
	}
	if val < 1 {
		return 0, errors.New("invalid poll interval: must be >= 1 second")
	}
	if int64(val) > maxPollIntervalSeconds {
		return 0, errors.New("invalid poll interval: exceeds supported duration")
	}
	return val, nil
}

func isFlag(arg string) bool {
	return (strings.HasPrefix(arg, "--") && arg != "--") || arg == "-h" || arg == "-v"
}
