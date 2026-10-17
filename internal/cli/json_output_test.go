package cli

import "testing"

func TestParseCommandRejectsOptionAsJSONOutputPath(t *testing.T) {
	_, err := ParseCommand([]string{"--json-output", "--no-json-output", "--", "env"})
	if err != ErrMissingFlagValue {
		t.Fatalf("expected missing JSON output value, got %v", err)
	}
}
