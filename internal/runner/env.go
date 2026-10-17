package runner

import (
	"strings"

	"github.com/neith-config/neith-agent/internal/logger"
)

// appendEnvVar appends or replaces an environment variable
func appendEnvVar(baseEnv []string, key, value string) []string {
	if !isValidEnvNameASCII(key) {
		return baseEnv
	}

	keyPrefix := key + "="
	for i, env := range baseEnv {
		if strings.HasPrefix(env, keyPrefix) {
			// In-place replacement
			baseEnv[i] = keyPrefix + value
			return baseEnv
		}
	}

	// Append new var
	return append(baseEnv, keyPrefix+value)
}

// envMerger provides reusable environment merging functionality
type envMerger struct {
	buffer   []string       // Reusable buffer
	keyIndex map[string]int // Reusable key->index mapping
}

// newEnvMerger creates a reusable merger
func newEnvMerger() *envMerger {
	return &envMerger{
		buffer:   make([]string, 0, 64), // Reasonable initial capacity
		keyIndex: make(map[string]int, 32),
	}
}

// merge performs environment merge with reusable structures
func (e *envMerger) merge(baseEnv []string, extra map[string]string, lg logger.Logger) []string {
	if len(extra) == 0 {
		return baseEnv
	}

	// Reset reusable structures
	e.buffer = e.buffer[:0]
	for k := range e.keyIndex {
		delete(e.keyIndex, k)
	}

	// Copy base env and build lookup map
	e.buffer = append(e.buffer, baseEnv...)
	for i, env := range baseEnv {
		if idx := strings.IndexByte(env, '='); idx > 0 {
			e.keyIndex[env[:idx]] = i
		}
	}

	// Process extra vars
	for key, value := range extra {
		if !isValidEnvNameASCII(key) {
			lg.Warning("Invalid environment variable name: " + key)
			continue
		}

		if idx, exists := e.keyIndex[key]; exists {
			// Replace existing
			e.buffer[idx] = key + "=" + value
		} else {
			// Append new
			e.buffer = append(e.buffer, key+"="+value)
			e.keyIndex[key] = len(e.buffer) - 1
		}
	}

	return e.buffer
}

// Global pool for shared use
var envMergerPool = newEnvMerger()

// isValidEnvNameASCII validates environment variable names for ASCII strings
func isValidEnvNameASCII(name string) bool {
	if len(name) == 0 {
		return false
	}

	first := name[0]
	if !((first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}

	return true
}

func MergeEnv(baseEnv []string, extra map[string]string, lg logger.Logger) []string {
	if len(extra) == 0 {
		return baseEnv
	}

	if len(extra) == 1 {
		// Single var optimization
		for k, v := range extra {
			return appendEnvVar(baseEnv, k, v)
		}
	}

	// Use pooled merger for multiple vars
	return envMergerPool.merge(baseEnv, extra, lg)
}
