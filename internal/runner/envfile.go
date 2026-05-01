package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var userConfigDir = os.UserConfigDir
var mkdirAll = os.MkdirAll
var openFile = os.OpenFile

func defaultEnvFilePath() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "neth-agent", "env.json"), nil
}

func writeEnvFile(path string, env map[string]string) error {
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(env); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

func ensureWritableEnvFile(path string) error {
	if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := openFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}
