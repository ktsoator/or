// Package workspace owns the directories a coding session runs in: the project
// roots a user registers, and the scratch directories the server generates for
// standalone chats. Path cleaning lives here alongside the scratch guard that
// depends on it, because that guard is the only thing standing between forged
// session metadata and a recursive delete of a real project.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalid reports a path that cannot be used as a workspace root.
var ErrInvalid = errors.New("workspace: invalid workspace")

// Clean resolves a path to an absolute, symlink-free, lexically clean form.
// Resolution failures are tolerated so a path that does not exist yet still
// normalizes; callers that require an existing directory use Validate.
func Clean(path string) (string, error) {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	return filepath.Clean(abs), nil
}

// Validate cleans path and proves it is an existing directory.
func Validate(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalid)
	}
	cleaned, err := Clean(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrInvalid, cleaned)
	}
	return cleaned, nil
}
