//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"testing"
)

func TestDefaultSignalDependencies(t *testing.T) {
	cmd := &exec.Cmd{Process: &os.Process{}}
	if got := getProcess(cmd); got != cmd.Process {
		t.Fatal("default getProcess did not return the command process")
	}
	if err := processSignal(&os.Process{}, fakeSignal{}); err == nil {
		t.Fatal("default processSignal unexpectedly succeeded")
	}
}
