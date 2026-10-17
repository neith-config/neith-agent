//go:build !windows

package agent

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// VersionFileOpenFlags returns platform flags that prevent following a
// pre-existing symlink when opening the persistent version file.
func VersionFileOpenFlags() int {
	return unix.O_NOFOLLOW
}

var unixFstat = unix.Fstat

// ValidateVersionFile rejects non-regular files and files owned by another
// local principal before the version store maps them.
func ValidateVersionFile(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat version file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("version file is not regular")
	}

	var stat unix.Stat_t
	if err := unixFstat(int(f.Fd()), &stat); err != nil {
		return fmt.Errorf("stat version file owner: %w", err)
	}
	if stat.Uid != uint32(unix.Getuid()) {
		return fmt.Errorf("version file is owned by uid %d, current uid is %d", stat.Uid, unix.Getuid())
	}
	return nil
}
