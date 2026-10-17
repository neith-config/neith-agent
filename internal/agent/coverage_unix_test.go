//go:build !windows

package agent

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestValidateVersionFileOwnerBranches(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "version")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := unixFstat
	defer func() { unixFstat = old }()
	unixFstat = func(int, *unix.Stat_t) error { return errors.New("fstat") }
	if err := ValidateVersionFile(f); err == nil {
		t.Fatal("expected fstat error")
	}
	unixFstat = func(_ int, stat *unix.Stat_t) error { stat.Uid = ^uint32(0); return nil }
	if err := ValidateVersionFile(f); err == nil {
		t.Fatal("expected foreign owner error")
	}
}
