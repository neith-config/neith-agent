//go:build !windows

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestWriteConfigAtomicallyUsesPrivateMode(t *testing.T) {
	previousUmask := unix.Umask(0o022)
	defer unix.Umask(previousUmask)

	a, _, _, _, _ := newTestAgent(AgentConfig{ServiceName: "private-mode"})
	a.configFile = filepath.Join(t.TempDir(), "snapshot.json")

	if err := a.writeConfigAtomically(map[string]string{"SECRET": "value"}); err != nil {
		t.Fatalf("writeConfigAtomically: %v", err)
	}

	info, err := os.Stat(a.configFile)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("snapshot mode = %04o, want 0600", got)
	}
}
