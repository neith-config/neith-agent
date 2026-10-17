//go:build windows

package agent

import (
	"fmt"
	"os"
)

// VersionFileOpenFlags returns platform flags that prevent following a
// pre-existing symlink when opening the persistent version file. Windows has
// no equivalent flag in this cross-build-only path.
func VersionFileOpenFlags() int { return 0 }

// ValidateVersionFile rejects non-regular files before the version store maps
// them. Native Windows ownership enforcement is outside the supported scope.
func ValidateVersionFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat version file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("version file is not regular")
	}
	return nil
}
