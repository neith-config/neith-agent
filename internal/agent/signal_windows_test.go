//go:build windows

package agent

// signalTestProcess is unreachable on Windows because TestAgentRunSignal skips
// there, but keeps the test package buildable on that platform.
func signalTestProcess(int) {}
