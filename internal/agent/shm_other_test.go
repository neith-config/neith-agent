//go:build !linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShmDir(t *testing.T) {
	dir := shmDir()
	expected := filepath.Join(os.TempDir(), "neith")
	if dir != expected {
		t.Fatalf("expected %s, got %s", expected, dir)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("expected shmDir to create the directory")
	}
}
