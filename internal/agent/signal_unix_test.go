//go:build !windows

package agent

import "syscall"

var syscallKill = syscall.Kill

func signalTestProcess(pid int) {
	_ = syscallKill(pid, syscall.SIGTERM)
}
