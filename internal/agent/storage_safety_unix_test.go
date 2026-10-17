//go:build !windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteConfigAtomicallyRejectsPreexistingTemporarySymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "storage-safety"})
	a.configFile = filepath.Join(dir, "snapshot.json")
	if err := os.Symlink(target, a.configFile+".tmp"); err != nil {
		t.Fatalf("create temporary symlink: %v", err)
	}

	if err := a.writeConfigAtomically(map[string]string{"SECRET": "new"}); err == nil {
		t.Fatal("expected pre-existing temporary symlink to be rejected")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("symlink target changed to %q", data)
	}
}
