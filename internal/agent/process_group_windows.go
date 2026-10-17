//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func configureChildProcess(cmd *exec.Cmd) {
}

func signalChildProcess(process *os.Process, sig os.Signal) error {
	return process.Signal(sig)
}

func killChildProcess(process *os.Process) error {
	return process.Kill()
}
