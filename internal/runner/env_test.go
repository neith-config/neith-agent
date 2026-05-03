package runner

import (
	"bytes"
	"testing"
)

type testLogger struct {
	warnings []string
	errors   []string
}

func (tl *testLogger) Debug(msg string)            {}
func (tl *testLogger) Info(msg string)             {}
func (tl *testLogger) Warning(msg string)          { tl.warnings = append(tl.warnings, msg) }
func (tl *testLogger) Error(msg string, err error) { tl.errors = append(tl.errors, msg) }

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	base := []string{"A=1", "B=2"}
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

	// Extra entries override base entries for the same key.
	base := []string{"A=old1", "B=old2"}
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

	base := []string{"A=1"}
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
