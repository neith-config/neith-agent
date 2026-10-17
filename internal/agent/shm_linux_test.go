//go:build linux

package agent

import "testing"

func TestShmDir(t *testing.T) {
	if shmDir() != "/dev/shm" {
		t.Fatalf("expected /dev/shm, got %s", shmDir())
	}
}
