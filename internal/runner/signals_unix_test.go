//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

type fakeSignal struct{}

func (fakeSignal) String() string { return "fake" }
func (fakeSignal) Signal()        {}

func TestForwardSignals_Unix(t *testing.T) {

	oldNotify := signalNotify
	oldStop := signalStop
	oldKill := syscallKill
	oldProcessSignal := processSignal
	oldGetProcess := getProcess
	defer func() {
		signalNotify = oldNotify
		signalStop = oldStop
		syscallKill = oldKill
		processSignal = oldProcessSignal
		getProcess = oldGetProcess
	}()

	getProcess = func(cmd *exec.Cmd) *os.Process { return cmd.Process }

	var ch chan<- os.Signal
	var capturedCap int
	stopped := false
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) {
		ch = c
		capturedCap = cap(c)
	}
	signalStop = func(_ chan<- os.Signal) { stopped = true }

	killed := false
	var killedPid int
	var killedSig syscall.Signal
	syscallKill = func(pid int, sig syscall.Signal) error {
		killed = true
		killedPid = pid
		killedSig = sig
		return nil
	}

	signaled := false
	processSignal = func(_ *os.Process, _ os.Signal) error {
		signaled = true
		return nil
	}

	cmd := &exec.Cmd{Process: &os.Process{Pid: 123}}
	cleanup := forwardSignals(cmd)
	notifyProcessStarted(cmd)

	ch <- syscall.SIGTERM
	cleanup()

	if !stopped {
		t.Fatalf("expected stop")
	}
	if !killed {
		t.Fatalf("expected kill")
	}
	if killedPid != -123 {
		t.Fatalf("expected -123, got %d", killedPid)
	}
	if killedSig != syscall.SIGTERM {
		t.Fatalf("expected SIGTERM")
	}
	if signaled {
		t.Fatalf("did not expect processSignal")
	}
	if capturedCap != 16 {
		t.Fatalf("expected buffer 16, got %d", capturedCap)
	}
}

func TestForwardSignals_Unix_NonSyscallSignal(t *testing.T) {

	oldNotify := signalNotify
	oldStop := signalStop
	oldKill := syscallKill
	oldProcessSignal := processSignal
	oldGetProcess := getProcess
	defer func() {
		signalNotify = oldNotify
		signalStop = oldStop
		syscallKill = oldKill
		processSignal = oldProcessSignal
		getProcess = oldGetProcess
	}()

	getProcess = func(cmd *exec.Cmd) *os.Process { return cmd.Process }

	var ch chan<- os.Signal
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) { ch = c }
	signalStop = func(chan<- os.Signal) {}

	killCalled := false
	syscallKill = func(int, syscall.Signal) error { killCalled = true; return nil }

	called := false
	processSignal = func(_ *os.Process, _ os.Signal) error {
		called = true
		return nil
	}

	cmd := &exec.Cmd{Process: &os.Process{Pid: 123}}
	cleanup := forwardSignals(cmd)
	notifyProcessStarted(cmd)

	ch <- fakeSignal{}
	ch <- syscall.SIGTERM
	cleanup()

	if !called {
		t.Fatalf("expected processSignal")
	}
	if !killCalled {
		t.Fatalf("expected kill after non-syscall signal")
	}
}

func TestForwardSignals_NilCmd(t *testing.T) {

	cleanup := forwardSignals(nil)
	cleanup()
}

func TestForwardSignals_Unix_NilProcess(t *testing.T) {

	oldNotify := signalNotify
	oldStop := signalStop
	oldKill := syscallKill
	oldProcessSignal := processSignal
	defer func() {
		signalNotify = oldNotify
		signalStop = oldStop
		syscallKill = oldKill
		processSignal = oldProcessSignal
	}()

	var ch chan<- os.Signal
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) { ch = c }
	signalStop = func(chan<- os.Signal) {}

	killCalled := false
	syscallKill = func(int, syscall.Signal) error { killCalled = true; return nil }
	processSignal = func(*os.Process, os.Signal) error { t.Fatalf("unexpected processSignal"); return nil }

	getProcess = func(cmd *exec.Cmd) *os.Process { return cmd.Process }

	cmd := &exec.Cmd{Process: &os.Process{Pid: 123}}
	state := 0
	getProcess = func(*exec.Cmd) *os.Process {
		if state == 0 {
			state++
			return nil
		}
		return cmd.Process
	}
	cleanup := forwardSignals(cmd)
	notifyProcessStarted(cmd)

	ch <- syscall.SIGTERM
	ch <- syscall.SIGTERM
	cleanup()

	if !killCalled {
		t.Fatalf("expected kill")
	}
}

func TestForwardSignals_Unix_NilProcessThenNonSyscall(t *testing.T) {

	oldNotify := signalNotify
	oldStop := signalStop
	oldKill := syscallKill
	oldProcessSignal := processSignal
	defer func() {
		signalNotify = oldNotify
		signalStop = oldStop
		syscallKill = oldKill
		processSignal = oldProcessSignal
	}()

	var ch chan<- os.Signal
	signalNotify = func(c chan<- os.Signal, _ ...os.Signal) { ch = c }
	signalStop = func(chan<- os.Signal) {}

	killCalled := false
	syscallKill = func(int, syscall.Signal) error { killCalled = true; return nil }
	processCalled := false
	processSignal = func(*os.Process, os.Signal) error { processCalled = true; return nil }

	getProcess = func(cmd *exec.Cmd) *os.Process { return cmd.Process }

	cmd := &exec.Cmd{Process: &os.Process{Pid: 123}}
	state := 0
	getProcess = func(*exec.Cmd) *os.Process {
		if state == 0 {
			state++
			return nil
		}
		return cmd.Process
	}
	cleanup := forwardSignals(cmd)
	notifyProcessStarted(cmd)

	ch <- syscall.SIGTERM
	ch <- fakeSignal{}
	cleanup()

	if !processCalled {
		t.Fatalf("expected processSignal")
	}
	if killCalled {
		t.Fatalf("did not expect kill")
	}
}
