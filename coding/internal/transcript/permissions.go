package transcript

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	privateDirectoryMode os.FileMode = 0o700
	privateFileMode      os.FileMode = 0o600
)

// SecurePrivatePermissions enforces private modes for every transcript JSONL
// file in dir, including files that remain lazily unloaded.
func SecurePrivatePermissions(dir string) error {
	if err := os.MkdirAll(dir, privateDirectoryMode); err != nil {
		return err
	}
	if err := os.Chmod(dir, privateDirectoryMode); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		if err := os.Chmod(filepath.Join(dir, entry.Name()), privateFileMode); err != nil {
			return err
		}
	}
	return nil
}

func ensurePrivateDirectory(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, privateDirectoryMode); err != nil {
		return err
	}
	return os.Chmod(dir, privateDirectoryMode)
}

// secureExistingFile enforces private modes before a transcript is read.
func secureExistingFile(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := os.Chmod(filepath.Dir(path), privateDirectoryMode); err != nil {
		return false, err
	}
	if err := os.Chmod(path, privateFileMode); err != nil {
		return false, err
	}
	return true, nil
}
