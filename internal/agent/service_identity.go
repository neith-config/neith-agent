package agent

import "fmt"

// ValidateServiceName accepts the single path component used to derive Neith's
// private snapshot and version files.
func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name must not be empty")
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return fmt.Errorf("service name contains invalid character %q", c)
	}
	return nil
}
