package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeEmptyAndInvalidNames(t *testing.T) {
	lg := &testLogger{}
	base := []string{"A=1"}
	if got := newEnvMerger().merge(base, map[string]string{}, lg); len(got) != len(base) {
		t.Fatalf("empty merger changed environment: %#v", got)
	}
	if isValidEnvNameASCII("") || isValidEnvNameASCII("1BAD") {
		t.Fatal("invalid environment name accepted")
	}
	if got := MergeEnv(base, map[string]string{}, lg); len(got) != len(base) {
		t.Fatalf("empty MergeEnv changed environment: %#v", got)
	}
}

func TestMergeEnvSingleKeyPaths(t *testing.T) {
	got := MergeEnv([]string{"A=old"}, map[string]string{"A": "new"}, &testLogger{})
	if len(got) != 1 || got[0] != "A=new" {
		t.Fatalf("single-key overlay did not replace existing value: %#v", got)
	}

	got = MergeEnv([]string{"A=old"}, map[string]string{"B": "new"}, &testLogger{})
	if len(got) != 2 || got[1] != "B=new" {
		t.Fatalf("single-key overlay did not append new value: %#v", got)
	}
}

func TestWriteEnvFileEncodeError(t *testing.T) {
	oldOpenFile := openFile
	defer func() { openFile = oldOpenFile }()

	file, err := os.CreateTemp(t.TempDir(), "env")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	openFile = func(string, int, os.FileMode) (*os.File, error) { return file, nil }
	if err := writeEnvFile(filepath.Join(t.TempDir(), "env.json"), map[string]string{"A": "1"}); err == nil {
		t.Fatal("expected closed-file encoding error")
	}
}

func TestExportedEnvFileContracts(t *testing.T) {
	oldUserConfigDir := userConfigDir
	defer func() { userConfigDir = oldUserConfigDir }()

	configDir := t.TempDir()
	userConfigDir = func() (string, error) { return configDir, nil }

	path, err := DefaultEnvFilePath()
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(configDir, "neith-agent", "env.json")
	if path != wantPath {
		t.Fatalf("DefaultEnvFilePath() = %q, want %q", path, wantPath)
	}

	output := filepath.Join(t.TempDir(), "env.json")
	if err := WriteEnvFile(output, map[string]string{"A": "1"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["A"] != "1" {
		t.Fatalf("WriteEnvFile() wrote %#v", got)
	}
}
