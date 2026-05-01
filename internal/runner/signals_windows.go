//go:build windows

package runner

import (
	"os"
	"os/exec"
	"os/signal"
)

var signalNotify = signal.Notify
var signalStop = signal.Stop
var processSignal = func(p *os.Process, sig os.Signal) error { return p.Signal(sig) }

func notifyProcessStarted(cmd *exec.Cmd) {
}

func forwardSignals(cmd *exec.Cmd) func() {
	if cmd == nil {
		return func() {}
	}

	sigCh := make(chan os.Signal, 16)
	// Windows meaningfully supports only os.Interrupt in Go's signal package.
	signalNotify(sigCh, os.Interrupt)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for sig := range sigCh {
			if cmd.Process == nil {
				continue
			}
			_ = processSignal(cmd.Process, sig)
		}
	}()

	return func() {
		signalStop(sigCh)
		close(sigCh)
		<-done
	}
}
