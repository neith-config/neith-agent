//go:build !windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVersionFileOpenFlagsRejectSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, make([]byte, 8), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	path := filepath.Join(dir, "version")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|VersionFileOpenFlags(), 0o600)
	if err == nil {
		_ = file.Close()
		t.Fatal("opened version-file symlink")
	}
}

func TestValidateVersionFileRejectsNonRegularFile(t *testing.T) {
	file, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open directory: %v", err)
	}
	defer file.Close()

	if err := ValidateVersionFile(file); err == nil {
		t.Fatal("accepted directory as version file")
	}
}
