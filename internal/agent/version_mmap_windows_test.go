//go:build windows

package agent

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
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
	f.Close()

	_, err = NewVersionStore(f)
	if err == nil {
		t.Fatal("expected error for closed file, got nil")
	}
}
