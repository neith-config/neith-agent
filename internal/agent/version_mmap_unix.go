//go:build !windows

package agent

import (
	"fmt"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

var unixMmap = unix.Mmap
var unixMunmap = unix.Munmap

type unixVersionStore struct {
	f    *os.File
	data []byte
}

// NewVersionStore opens path as an mmap-backed 8-byte version counter.
// f must be opened with os.O_RDWR (and O_CREATE if new). Caller passes the
// already-opened file so the constructor does not create its own dependencies.
func NewVersionStore(f *os.File) (VersionStore, error) {
	if err := f.Truncate(8); err != nil {
		return nil, fmt.Errorf("truncate version file: %w", err)
	}
	data, err := unixMmap(int(f.Fd()), 0, 8, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap version file: %w", err)
	}
	return &unixVersionStore{f: f, data: data}, nil
}

func (s *unixVersionStore) Store(version int64) {
	atomic.StoreInt64((*int64)(unsafe.Pointer(&s.data[0])), version)
}

func (s *unixVersionStore) Close() error {
	if err := unixMunmap(s.data); err != nil {
		return fmt.Errorf("munmap version file: %w", err)
	}
	return s.f.Close()
}
