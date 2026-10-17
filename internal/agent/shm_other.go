//go:build !linux

package agent

import (
	"os"
	"path/filepath"
)

func shmDir() string {
	dir := filepath.Join(os.TempDir(), "neith")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}
