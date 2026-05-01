package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultEnvFilePath(t *testing.T) {
	old := userConfigDir
	defer func() { userConfigDir = old }()

	tmp := t.TempDir()
	userConfigDir = func() (string, error) {
		return filepath.Join(tmp, "config"), nil
	}

	got, err := defaultEnvFilePath()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := filepath.Join(tmp, "config", "neth-agent", "env.json")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDefaultEnvFilePathError(t *testing.T) {
	old := userConfigDir
	defer func() { userConfigDir = old }()

	userConfigDir = func() (string, error) { return "", errors.New("boom") }
	if _, err := defaultEnvFilePath(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env.json")

	env := map[string]string{"A": "1"}
	if err := writeEnvFile(path, env); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json err: %v", err)
	}
	if decoded["A"] != "1" {
		t.Fatalf("unexpected decoded: %#v", decoded)
	}
	if string(data) != "{\n  \"A\": \"1\"\n}\n" {
		t.Fatalf("expected pretty JSON, got %q", string(data))
	}
}

func TestWriteEnvFileError(t *testing.T) {
	oldMkdir := mkdirAll
	oldOpen := openFile
	defer func() {
		mkdirAll = oldMkdir
		openFile = oldOpen
	}()

	testPath := filepath.Join(t.TempDir(), "subdir", "env.json")

	mkdirAll = func(string, os.FileMode) error { return errors.New("boom") }
	if err := writeEnvFile(testPath, map[string]string{}); err == nil {
		t.Fatalf("expected error")
	}

	mkdirAll = oldMkdir
	openErr := errors.New("boom")
	openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, openErr }
	if err := writeEnvFile(testPath, map[string]string{}); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestEnsureWritableEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env.json")
	if err := ensureWritableEnvFile(path); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestEnsureWritableEnvFileError(t *testing.T) {
	oldMkdir := mkdirAll
	oldOpen := openFile
	defer func() {
		mkdirAll = oldMkdir
		openFile = oldOpen
	}()

	testPath := filepath.Join(t.TempDir(), "subdir", "env.json")

	mkdirAll = func(string, os.FileMode) error { return errors.New("boom") }
	if err := ensureWritableEnvFile(testPath); err == nil {
		t.Fatalf("expected error")
	}

	mkdirAll = oldMkdir
	openErr := errors.New("boom")
	openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, openErr }
	if err := ensureWritableEnvFile(testPath); !errors.Is(err, openErr) {
		t.Fatalf("expected open error, got %v", err)
	}
}
