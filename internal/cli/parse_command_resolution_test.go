package cli

import "testing"

func TestParseCommandSupportsDaemonModeWithoutCommand(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		nil,
		{"--hub-url", "http://hub"},
		{"--token", "tok", "--poll-interval", "10"},
	} {
		if opts, err := ParseCommand(args); err != nil {
			t.Fatalf("ParseCommand(%#v): %v", args, err)
		} else if opts.Command != "" || len(opts.CommandArgs) != 0 {
			t.Fatalf("ParseCommand(%#v) returned child command: %#v", args, opts)
		}
	}
}
