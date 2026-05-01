package runner

import (
	"bytes"
	"testing"
)

type testLogger struct {
	warnings []string
	errors   []string
}

func (tl *testLogger) Debug(msg string)              {}
func (tl *testLogger) Info(msg string)               {}
func (tl *testLogger) Warning(msg string)            { tl.warnings = append(tl.warnings, msg) }
func (tl *testLogger) Error(msg string, err error)   { tl.errors = append(tl.errors, msg) }

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	base := map[string]string{"A": "1", "B": "2"}
	extra := map[string]string{"B": "3", "C": "4", "D": "5"}

	got := MergeEnv(base, extra, &testLogger{})

	wantA := "A=1"
	wantB := "B=3"
	wantC := "C=4"
	wantD := "D=5"

	seen := map[string]bool{}
	for _, kv := range got {
		seen[kv] = true
	}

	if !seen[wantA] || !seen[wantB] || !seen[wantC] || !seen[wantD] {
		t.Fatalf("unexpected env: %#v", got)
	}
	if seen["B=2"] {
		t.Fatalf("expected B to be overridden: %#v", got)
	}
}

func TestMergeEnv_ExtraOverridesBase(t *testing.T) {
	t.Parallel()

	// Maps cannot have duplicate keys, so with map-based implementation,
	// extra entries completely replace base entries for the same key.
	base := map[string]string{"A": "old1", "B": "old2"}
	extra := map[string]string{"A": "new"}

	got := MergeEnv(base, extra, &testLogger{})

	seen := map[string]bool{}
	for _, kv := range got {
		seen[kv] = true
	}

	if !seen["A=new"] {
		t.Fatalf("expected A to be overridden by extra, got %#v", got)
	}
	if !seen["B=old2"] {
		t.Fatalf("expected B from base to remain, got %#v", got)
	}
	if seen["A=old1"] {
		t.Fatalf("expected A=old1 to not exist, got %#v", got)
	}
}

func TestMergeEnv_SkipsInvalidNames(t *testing.T) {
	t.Parallel()

	tl := &testLogger{}

	base := map[string]string{"A": "1"}
	extra := map[string]string{
		"B":      "2",
		"123":    "invalid",
		"C-D":    "invalid",
		"C.D":    "invalid",
		"_VALID": "3",
	}

	got := MergeEnv(base, extra, tl)

	seen := map[string]bool{}
	for _, kv := range got {
		seen[kv] = true
	}

	if !seen["A=1"] || !seen["B=2"] || !seen["_VALID=3"] {
		t.Fatalf("unexpected env: %#v", got)
	}

	if seen["123=invalid"] || seen["C-D=invalid"] || seen["C.D=invalid"] {
		t.Fatalf("expected invalid names to be skipped: %#v", got)
	}

	// Verify warnings were logged for invalid names
	if len(tl.warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %v", len(tl.warnings), tl.warnings)
	}

	warningStr := ""
	for _, w := range tl.warnings {
		warningStr += w + "\n"
	}

	if !bytes.Contains([]byte(warningStr), []byte("123")) {
		t.Fatalf("expected warning for invalid name '123', got: %s", warningStr)
	}
	if !bytes.Contains([]byte(warningStr), []byte("C-D")) {
		t.Fatalf("expected warning for invalid name 'C-D', got: %s", warningStr)
	}
	if !bytes.Contains([]byte(warningStr), []byte("C.D")) {
		t.Fatalf("expected warning for invalid name 'C.D', got: %s", warningStr)
	}
}

func TestEnvToMap(t *testing.T) {
	t.Parallel()

	envSlice := []string{"A=1", "B=2", "C=value with equals=sign", "malformed"}

	got := envToMap(envSlice)

	if got["A"] != "1" {
		t.Fatalf("expected A=1, got %q", got["A"])
	}
	if got["B"] != "2" {
		t.Fatalf("expected B=2, got %q", got["B"])
	}
	if got["C"] != "value with equals=sign" {
		t.Fatalf("expected C=value with equals=sign, got %q", got["C"])
	}
	if _, ok := got["malformed"]; ok {
		t.Fatalf("expected malformed entry to be skipped, got %#v", got)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
}

func TestIsValidEnvName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid_uppercase", input: "FOO", want: true},
		{name: "valid_lowercase", input: "foo", want: true},
		{name: "valid_mixed", input: "FooBar", want: true},
		{name: "valid_with_numbers", input: "FOO123", want: true},
		{name: "valid_with_underscore", input: "_FOO", want: true},
		{name: "valid_underscore_and_numbers", input: "_FOO_123", want: true},
		{name: "invalid_empty", input: "", want: false},
		{name: "invalid_starts_with_number", input: "123FOO", want: false},
		{name: "invalid_contains_hyphen", input: "FOO-BAR", want: false},
		{name: "invalid_contains_dot", input: "FOO.BAR", want: false},
		{name: "invalid_contains_space", input: "FOO BAR", want: false},
		{name: "invalid_contains_equals", input: "FOO=BAR", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidEnvName(tt.input)
			if got != tt.want {
				t.Errorf("isValidEnvName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
