package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scratch generates and reclaims the server-owned directories that standalone
// chat sessions run in. These are not registered projects: they are never
// listed in the sidebar and are created and destroyed with their session.
type Scratch struct {
	root string
}

// NewScratch returns the scratch manager rooted under the product data
// directory. Every generated path lives exactly two levels below root.
func NewScratch(dataDir string) *Scratch {
	return &Scratch{root: filepath.Join(dataDir, "workspaces")}
}

// Create makes the directory tree for one session and returns its cleaned
// path. A partially created tree is removed rather than left behind.
func (s *Scratch) Create(sessionID string, startedAt time.Time) (string, error) {
	path := filepath.Join(s.root, startedAt.Format("2006-01-02"), sessionID)
	if err := EnsureDirectories(path); err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	cleaned, err := s.Validate(path)
	if err != nil {
		_ = os.RemoveAll(path)
		return "", err
	}
	return cleaned, nil
}

// Validate proves that path is one generated session directory exactly two
// levels below the managed root. This guard is required before any recursive
// cleanup, so an external project can never be removed through forged session
// metadata.
func (s *Scratch) Validate(path string) (string, error) {
	root, err := Clean(s.root)
	if err != nil {
		return "", err
	}
	cleaned, err := Clean(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, cleaned)
	if err != nil || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: scratch workspace is outside managed storage", ErrInvalid)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == ".." {
		return "", fmt.Errorf("%w: invalid scratch workspace path", ErrInvalid)
	}
	return cleaned, nil
}

// Remove deletes one scratch directory after proving it is managed storage.
func (s *Scratch) Remove(path string) error {
	cleaned, err := s.Validate(path)
	if err != nil {
		return err
	}
	return os.RemoveAll(cleaned)
}

// EnsureDirectories creates the fixed layout every scratch workspace has. It is
// idempotent so a restored session can repair a partially deleted tree.
func EnsureDirectories(path string) error {
	for _, directory := range []string{path, filepath.Join(path, "work"), filepath.Join(path, "outputs")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("workspace: create scratch workspace: %w", err)
		}
	}
	return nil
}
