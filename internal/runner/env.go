package runner

import (
	"strings"

	"github.com/neith-config/neith-agent/internal/logger"
)

// appendEnvVar appends or replaces an environment variable.
func appendEnvVar(baseEnv []string, key, value string, lg logger.Logger) []string {
	if !isValidEnvEntry(key, value) {
		lg.Warning("Invalid environment variable: " + key)
		return baseEnv
	}

	keyPrefix := key + "="
	replaced := false
	for i, env := range baseEnv {
		if strings.HasPrefix(env, keyPrefix) {
			// Replace every duplicate so the single-key and multi-key paths have
			// the same last-value semantics when exec constructs the child env.
			baseEnv[i] = keyPrefix + value
			replaced = true
		}
	}
	if replaced {
		return baseEnv
	}

	// Append new var.
	return append(baseEnv, keyPrefix+value)
}

// envMerger performs environment merging with caller-owned per-call storage.
type envMerger struct{}

// newEnvMerger creates a stateless merger. Keeping the helper type preserves the
// focused merge contract while ensuring every call owns its returned slice and
// lookup map.
func newEnvMerger() *envMerger {
	return &envMerger{}
}

// merge performs environment merge using storage owned by this call.
func (e *envMerger) merge(baseEnv []string, extra map[string]string, lg logger.Logger) []string {
	if len(extra) == 0 {
		return baseEnv
	}

	buffer := make([]string, 0, len(baseEnv)+len(extra))
	keyIndex := make(map[string][]int, len(baseEnv)+len(extra))

	// Copy base env and build lookup map, retaining every duplicate index.
	buffer = append(buffer, baseEnv...)
	for i, env := range baseEnv {
		if idx := strings.IndexByte(env, '='); idx > 0 {
			key := env[:idx]
			keyIndex[key] = append(keyIndex[key], i)
		}
	}

	// Process extra vars.
	for key, value := range extra {
		if !isValidEnvEntry(key, value) {
			lg.Warning("Invalid environment variable: " + key)
			continue
		}

		if indices, exists := keyIndex[key]; exists {
			// Replace every duplicate base entry so the last value seen by exec
			// cannot override the configured overlay.
			for _, i := range indices {
				buffer[i] = key + "=" + value
			}
		} else {
			// Append new.
			buffer = append(buffer, key+"="+value)
			keyIndex[key] = []int{len(buffer) - 1}
		}
	}

	return buffer
}

// isValidEnvNameASCII validates shell-compatible ASCII environment names.
func isValidEnvNameASCII(name string) bool {
	if len(name) == 0 {
		return false
	}

	first := name[0]
	if !isASCIIEnvLetter(first) && first != '_' {
		return false
	}

	for i := 1; i < len(name); i++ {
		c := name[i]
		if !isASCIIEnvLetter(c) && !(c >= '0' && c <= '9') && c != '_' {
			return false
		}
	}

	return true
}

func isASCIIEnvLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isValidEnvEntry(key, value string) bool {
	return isValidEnvNameASCII(key) && !strings.ContainsRune(value, '\x00')
}

func MergeEnv(baseEnv []string, extra map[string]string, lg logger.Logger) []string {
	if len(extra) == 0 {
		return baseEnv
	}

	if len(extra) == 1 {
		// Single var optimization.
		for k, v := range extra {
			return appendEnvVar(baseEnv, k, v, lg)
		}
	}

	// Use caller-owned storage for multiple vars.
	return newEnvMerger().merge(baseEnv, extra, lg)
}
