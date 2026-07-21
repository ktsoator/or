package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Deleting a session renames its files aside before the index is rewritten, so
// a failed index write can put them back. Nothing is unlinked until the new
// index is durable.

type stagedFile struct {
	original string
	staged   string
}

func (m *Manager) sessionFiles(record record) ([]string, error) {
	transcript, err := filepath.Abs(record.Transcript)
	if err != nil {
		return nil, err
	}
	sessionDir, err := filepath.Abs(filepath.Dir(m.indexPath))
	if err != nil {
		return nil, err
	}
	if filepath.Dir(transcript) != sessionDir {
		return nil, fmt.Errorf("session: refusing to delete transcript outside session storage: %s", transcript)
	}
	details := strings.TrimSuffix(transcript, ".jsonl") + ".details.jsonl"
	return []string{transcript, details}, nil
}

func stageFiles(paths []string) ([]stagedFile, error) {
	var staged []stagedFile
	for _, path := range paths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			restoreFiles(staged)
			return nil, err
		}
		tombstone := path + ".deleted-" + NewID()
		if err := os.Rename(path, tombstone); err != nil {
			restoreFiles(staged)
			return nil, err
		}
		staged = append(staged, stagedFile{original: path, staged: tombstone})
	}
	return staged, nil
}

func restoreFiles(files []stagedFile) {
	for i := len(files) - 1; i >= 0; i-- {
		_ = os.Rename(files[i].staged, files[i].original)
	}
}

func removeStagedPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}
