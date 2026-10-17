//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRunOpensVersionFileWithoutFollowingSymlinks(t *testing.T) {
	oldOpen := osOpenFile
	defer func() { osOpenFile = oldOpen }()

	var gotFlags int
	osOpenFile = func(_ string, flags int, _ os.FileMode) (*os.File, error) {
		gotFlags = flags
		return nil, errors.New("stop after flag capture")
	}
	t.Setenv("NEITH_HUB_URL", "http://hub")
	t.Setenv("NEITH_TOKEN", "tok")

	var stderr bytes.Buffer
	if code := run([]string{"--", "echo", "hi"}, &stderr); code != 1 {
		t.Fatalf("expected setup failure, got %d", code)
	}
	if gotFlags&unix.O_NOFOLLOW == 0 {
		t.Fatalf("version file open flags %08x do not include O_NOFOLLOW", gotFlags)
	}
}
