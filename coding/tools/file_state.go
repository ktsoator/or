package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	// ErrFileNotRead means a mutating tool was asked to change an existing file
	// that has not been observed by Read in this tool-set lifetime.
	ErrFileNotRead = errors.New("file has not been read")
	// ErrFileChanged means the file's current disk version no longer matches the
	// version most recently observed or produced by the tools.
	ErrFileChanged = errors.New("file has changed since it was read")
)

// FileVersion is the portable, inexpensive identity used for optimistic file
// concurrency checks. It deliberately avoids hashing the full file so a small
// range read of a large file remains a range read.
type FileVersion struct {
	ModTime time.Time
	Size    int64
}

// FileStateStore tracks disk versions observed by one coding tool set. It is
// safe for concurrent reads while mutating tools execute sequentially.
type FileStateStore struct {
	mu    sync.RWMutex
	files map[string]FileVersion
}

func NewFileStateStore() *FileStateStore {
	return &FileStateStore{files: make(map[string]FileVersion)}
}

// Record marks path's current version as observed by the model or produced by a
// successful tool write.
func (s *FileStateStore) Record(path string, info os.FileInfo) {
	s.mu.Lock()
	s.files[fileStateKey(path)] = fileVersion(info)
	s.mu.Unlock()
}

// Check verifies that path was observed and still has the same disk version.
func (s *FileStateStore) Check(path string, info os.FileInfo) error {
	s.mu.RLock()
	observed, ok := s.files[fileStateKey(path)]
	s.mu.RUnlock()
	if !ok {
		return ErrFileNotRead
	}
	if !observed.Equal(fileVersion(info)) {
		return ErrFileChanged
	}
	return nil
}

// Delete forgets path after an operation changed it but its new version could
// not be observed. The next mutation must Read it again.
func (s *FileStateStore) Delete(path string) {
	s.mu.Lock()
	delete(s.files, fileStateKey(path))
	s.mu.Unlock()
}

func fileStateKey(path string) string { return filepath.Clean(path) }

func fileVersion(info os.FileInfo) FileVersion {
	return FileVersion{ModTime: info.ModTime(), Size: info.Size()}
}

func (v FileVersion) Equal(other FileVersion) bool {
	return v.Size == other.Size && v.ModTime.Equal(other.ModTime)
}

func sameFileVersion(a, b os.FileInfo) bool {
	return fileVersion(a).Equal(fileVersion(b))
}

func mutationStateError(tool, path string, err error) error {
	switch {
	case errors.Is(err, ErrFileNotRead):
		return fmt.Errorf("%s %s: %w; use read before changing it", tool, path, err)
	case errors.Is(err, ErrFileChanged):
		return fmt.Errorf("%s %s: %w; read it again before changing it", tool, path, err)
	default:
		return fmt.Errorf("%s %s: %w", tool, path, err)
	}
}
