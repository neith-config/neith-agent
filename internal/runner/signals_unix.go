//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
)

var signalNotify = signal.Notify
var signalStop = signal.Stop
var syscallKill = syscall.Kill
var processSignal = func(p *os.Process, sig os.Signal) error { return p.Signal(sig) }
var getProcess = func(cmd *exec.Cmd) *os.Process { return cmd.Process }

type signalForwarder struct {
	started atomic.Bool
	sigCh   chan os.Signal
	done    chan struct{}
}

func (sf *signalForwarder) notifyStarted() {
	sf.started.Store(true)
}

func (sf *signalForwarder) cleanup() {
	signalStop(sf.sigCh)
	close(sf.sigCh)
	<-sf.done
}

var activeForwarder atomic.Pointer[signalForwarder]

func notifyProcessStarted(cmd *exec.Cmd) {
	if sf := activeForwarder.Load(); sf != nil {
		sf.notifyStarted()
	}
}

func forwardSignals(cmd *exec.Cmd) func() {
	if cmd == nil {
		return func() {}
	}

	// Put child in its own process group so we can signal the whole group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	sf := &signalForwarder{
		sigCh: make(chan os.Signal, 16),
		done:  make(chan struct{}),
	}

	activeForwarder.Store(sf)

	signalNotify(sf.sigCh)

	go func() {
		defer close(sf.done)
		for sig := range sf.sigCh {
			if !sf.started.Load() {
				continue
			}

			p := getProcess(cmd)
			if p == nil {
				continue
			}

			s, ok := sig.(syscall.Signal)
			if !ok {
				_ = processSignal(p, sig)
				continue
			}

			// Best-effort: send to process group.
			_ = syscallKill(-p.Pid, s)
		}
	}()

	return func() {
		activeForwarder.Store(nil)
		sf.cleanup()
	}
}
