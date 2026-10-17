//go:build linux

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestAgentStopChildProcessTerminatesDescendants(t *testing.T) {
	a, _, _, _, lg := newTestAgent(AgentConfig{
		ServiceName: "process-group-test",
		Command:     "sh",
		Args:        []string{"-c", "trap '' TERM; (trap '' TERM; sleep 60) & wait"},
		StopDelay:   100 * time.Millisecond,
	})
	a.configFile = filepath.Join(t.TempDir(), "config.json")

	if err := a.startChildProcess(); err != nil {
		t.Fatalf("startChildProcess: %v", err)
	}

	a.childMu.Lock()
	parentPID := a.childProcess.Pid
	a.childMu.Unlock()

	waitDone := make(chan struct{})
	go func() {
		_ = a.waitForChildProcess()
		close(waitDone)
	}()
	t.Cleanup(func() {
		a.stopChildProcess()
		<-waitDone
	})

	descendantPID, err := waitForLinuxDescendant(parentPID, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	a.stopChildProcess()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("child process was not reaped after process-group shutdown")
	}

	if err := waitForLinuxProcessGone(descendantPID, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(lg.warnings) == 0 {
		t.Fatal("expected process-group shutdown to escalate to SIGKILL")
	}
}

func waitForLinuxDescendant(parentPID int, timeout time.Duration) (int, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pid, ok := readLinuxDescendant(parentPID); ok {
			return pid, nil
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return 0, fmt.Errorf("child process %d did not create a descendant", parentPID)
		}
	}
}

func readLinuxDescendant(parentPID int) (int, bool) {
	path := fmt.Sprintf("/proc/%d/task/%d/children", parentPID, parentPID)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for _, field := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(field)
		if err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}

func waitForLinuxProcessGone(pid int, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !linuxProcessLive(pid) {
			return nil
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return fmt.Errorf("descendant process %d remained alive after process-group shutdown", pid)
		}
	}
}

func linuxProcessLive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	return len(fields) > 2 && fields[2] != "Z" && syscall.Kill(pid, 0) == nil
}
