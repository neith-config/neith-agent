package runner

import (
	"strings"
	"unicode"

	"github.com/neth-config/neth-agent/internal/logger"
)

func envToMap(envSlice []string) map[string]string {
	result := make(map[string]string, len(envSlice))
	for _, kv := range envSlice {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			// Skip malformed entries without '='
			continue
		}
		key := kv[:idx]
		value := kv[idx+1:]
		result[key] = value
	}
	return result
}

func isValidEnvName(name string) bool {
	if len(name) == 0 {
		return false
	}

	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}

	return true
}

func MergeEnv(base map[string]string, extra map[string]string, lg logger.Logger) []string {
	result := make(map[string]string)

	// Copy all entries from base
	for k, v := range base {
		result[k] = v
	}

	// Copy all entries from extra (overwrites any duplicates)
	for k, v := range extra {
		if !isValidEnvName(k) {
			lg.Warning("Invalid environment variable name: " + k)
			continue
		}
		result[k] = v
	}

	// Convert map back to []string in "KEY=value" format
	output := make([]string, 0, len(result))
	for k, v := range result {
		output = append(output, k+"="+v)
	}

	return output
}

func keyFromEnvPair(kv string) string {
	idx := strings.IndexByte(kv, '=')
	if idx < 0 {
		return kv
	}
	return kv[:idx]
}
