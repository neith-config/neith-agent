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
	keyIndex := make(map[string]int, len(baseEnv)+len(extra))

	// Copy base env and build lookup map.
	buffer = append(buffer, baseEnv...)
	for i, env := range baseEnv {
		if idx := strings.IndexByte(env, '='); idx > 0 {
			keyIndex[env[:idx]] = i
		}
	}

	// Process extra vars.
	for key, value := range extra {
		if !isValidEnvNameASCII(key) {
			lg.Warning("Invalid environment variable name: " + key)
			continue
		}

		if idx, exists := keyIndex[key]; exists {
			// Replace existing.
			buffer[idx] = key + "=" + value
		} else {
			// Append new.
			buffer = append(buffer, key+"="+value)
			keyIndex[key] = len(buffer) - 1
		}
	}

	return buffer
}

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

	// Use caller-owned storage for multiple vars.
	return newEnvMerger().merge(baseEnv, extra, lg)
}
