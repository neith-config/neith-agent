package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestVersionFileChmodError(t *testing.T) {
	oldOpen := osOpenFile
	oldChmod := versionFileChmod
	defer func() {
		osOpenFile = oldOpen
		versionFileChmod = oldChmod
	}()
	f, err := os.CreateTemp(t.TempDir(), "version")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	osOpenFile = func(string, int, os.FileMode) (*os.File, error) { return f, nil }
	versionFileChmod = func(*os.File, os.FileMode) error { return errors.New("chmod") }
	t.Setenv("NEITH_TOKEN", "token")
	var stderr bytes.Buffer
	if code := run([]string{"--", "true"}, &stderr); code != 1 {
		t.Fatalf("got %d, want 1", code)
	}
}
