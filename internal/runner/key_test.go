package runner

import "testing"

func TestKeyFromEnvPair(t *testing.T) {
	t.Parallel()

	if keyFromEnvPair("A=1") != "A" {
		t.Fatalf("expected A")
	}
	if keyFromEnvPair("A") != "A" {
		t.Fatalf("expected A")
	}
	if keyFromEnvPair("=1") != "" {
		t.Fatalf("expected empty key")
	}
}
