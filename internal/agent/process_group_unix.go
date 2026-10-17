//go:build !windows

package agent

import (
	"os"
	"os/exec"
	"syscall"
)

func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalChildProcess(process *os.Process, sig os.Signal) error {
	if signal, ok := sig.(syscall.Signal); ok {
		return syscall.Kill(-process.Pid, signal)
	}
	return process.Signal(sig)
}

func killChildProcess(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}
