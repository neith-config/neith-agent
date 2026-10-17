package runner

import "testing"

func TestMergeEnvAcceptsLowercaseAndMixedCaseNames(t *testing.T) {
	t.Parallel()

	got := MergeEnv(
		[]string{"BASE=1"},
		map[string]string{"http_proxy": "http://proxy", "Path": "/srv/bin"},
		&testLogger{},
	)

	if !hasEnvEntry(got, "http_proxy=http://proxy") || !hasEnvEntry(got, "Path=/srv/bin") {
		t.Fatalf("expected valid shell environment names to be retained: %#v", got)
	}
}

func TestMergeEnvOverridesAllDuplicateBaseEntriesForSingleAndMultipleKeys(t *testing.T) {
	t.Parallel()

	for _, extra := range []map[string]string{
		{"DUP": "new"},
		{"DUP": "new", "OTHER": "value"},
	} {
		got := MergeEnv([]string{"DUP=old1", "DUP=old2", "BASE=keep"}, extra, &testLogger{})

		if !hasEnvEntry(got, "DUP=new") || hasEnvEntry(got, "DUP=old1") || hasEnvEntry(got, "DUP=old2") {
			t.Fatalf("expected every duplicate base entry to be overridden for extra=%#v: %#v", extra, got)
		}
	}
}

func TestMergeEnvRejectsNULInNameOrValue(t *testing.T) {
	t.Parallel()

	lg := &testLogger{}
	got := MergeEnv(
		[]string{"BASE=1"},
		map[string]string{"VALID": "bad\x00value", "BAD\x00NAME": "value"},
		lg,
	)

	if len(got) != 1 || got[0] != "BASE=1" {
		t.Fatalf("expected NUL-containing entries to be rejected: %#v", got)
	}
	if len(lg.warnings) != 2 {
		t.Fatalf("expected one warning per rejected entry, got %d: %v", len(lg.warnings), lg.warnings)
	}
}

func TestMergeEnvWarnsForInvalidSingleEntry(t *testing.T) {
	t.Parallel()

	lg := &testLogger{}
	got := MergeEnv([]string{"BASE=1"}, map[string]string{"BAD-NAME": "value"}, lg)

	if len(got) != 1 || got[0] != "BASE=1" {
		t.Fatalf("expected invalid entry to be rejected: %#v", got)
	}
	if len(lg.warnings) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(lg.warnings), lg.warnings)
	}
}
