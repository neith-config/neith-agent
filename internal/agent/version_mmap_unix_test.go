//go:build !windows

package agent

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNewVersionStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "version")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	store, err := NewVersionStore(f)
	if err != nil {
		t.Fatalf("NewVersionStore: %v", err)
	}
	defer store.Close()

	store.Store(42)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	got := int64(binary.LittleEndian.Uint64(data))
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestVersionStoreMultipleStores(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "v"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	store, err := NewVersionStore(f)
	if err != nil {
		t.Fatalf("NewVersionStore: %v", err)
	}
	defer store.Close()

	for i := int64(1); i <= 5; i++ {
		store.Store(i)
		data, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatalf("read file at %d: %v", i, err)
		}
		got := int64(binary.LittleEndian.Uint64(data))
		if got != i {
			t.Fatalf("expected %d, got %d", i, got)
		}
	}
}

func TestVersionStoreClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "v"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	store, err := NewVersionStore(f)
	if err != nil {
		t.Fatalf("NewVersionStore: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewVersionStoreErrorClosedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "v"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	f.Close() // close before passing — Truncate will fail

	_, err = NewVersionStore(f)
	if err == nil {
		t.Fatal("expected error for closed file, got nil")
	}
}

func TestNewVersionStoreMmapError(t *testing.T) {
	old := unixMmap
	defer func() { unixMmap = old }()
	unixMmap = func(fd int, offset int64, length int, prot int, flags int) ([]byte, error) {
		return nil, errors.New("injected mmap error")
	}

	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "v"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()

	_, err = NewVersionStore(f)
	if err == nil {
		t.Fatal("expected mmap error, got nil")
	}
}

func TestVersionStoreCloseMunmapError(t *testing.T) {
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "v"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}

	store, err := NewVersionStore(f)
	if err != nil {
		t.Fatalf("NewVersionStore: %v", err)
	}

	old := unixMunmap
	defer func() { unixMunmap = old }()
	unixMunmap = func(b []byte) error {
		// call real munmap so memory is cleaned up, but return error for test
		_ = unix.Munmap(b)
		return errors.New("injected munmap error")
	}

	if err := store.Close(); err == nil {
		t.Fatal("expected munmap error, got nil")
	}
}
