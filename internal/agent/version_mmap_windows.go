//go:build windows

package agent

import (
	"fmt"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsVersionStore struct {
	f      *os.File
	handle windows.Handle
	addr   uintptr
}

// NewVersionStore opens path as an mmap-backed 8-byte version counter.
// f must be opened with os.O_RDWR (and O_CREATE if new). Caller passes the
// already-opened file so the constructor does not create its own dependencies.
func NewVersionStore(f *os.File) (VersionStore, error) {
	if err := f.Truncate(8); err != nil {
		return nil, fmt.Errorf("truncate version file: %w", err)
	}
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READWRITE, 0, 8, nil)
	if err != nil {
		return nil, fmt.Errorf("create file mapping: %w", err)
	}
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_WRITE, 0, 0, 8)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("map view of file: %w", err)
	}
	return &windowsVersionStore{f: f, handle: h, addr: addr}, nil
}

func (s *windowsVersionStore) Current() int64 {
	return atomic.LoadInt64((*int64)(unsafe.Pointer(s.addr)))
}

func (s *windowsVersionStore) Store(version int64) {
	atomic.StoreInt64((*int64)(unsafe.Pointer(s.addr)), version)
}

func (s *windowsVersionStore) Close() error {
	if err := windows.UnmapViewOfFile(s.addr); err != nil {
		return fmt.Errorf("unmap view of file: %w", err)
	}
	if err := windows.CloseHandle(s.handle); err != nil {
		return fmt.Errorf("close file mapping handle: %w", err)
	}
	return s.f.Close()
}
