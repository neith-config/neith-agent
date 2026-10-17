//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func configureChildProcess(cmd *exec.Cmd) {
}

func signalChildProcess(process *os.Process, sig os.Signal) error {
	// Windows is retained for cross-build verification only. Go's generic
	// Process.Signal does not provide Unix-equivalent graceful process-group
	// delivery here.
	return process.Signal(sig)
}

func killChildProcess(process *os.Process) error {
	return process.Kill()
}
