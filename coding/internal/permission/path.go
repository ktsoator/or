package permission

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PathResolver classifies filesystem targets against one canonical workspace.
type PathResolver struct {
	workspace string
}

// NewPathResolver canonicalizes workspace, including symlinks when possible.
func NewPathResolver(workspace string) (PathResolver, error) {
	if strings.TrimSpace(workspace) == "" {
		return PathResolver{}, fmt.Errorf("permission: workspace path is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return PathResolver{}, fmt.Errorf("permission: resolve workspace: %w", err)
	}
	resolved, err := resolveAllowMissing(abs)
	if err != nil {
		return PathResolver{}, fmt.Errorf("permission: resolve workspace: %w", err)
	}
	return PathResolver{workspace: resolved}, nil
}

// Resolve enriches a filesystem access with its canonical target and scope.
// An uncertain target stays LocationUnknown so policy fails closed to Ask.
func (r PathResolver) Resolve(access Access) Access {
	if access.Action != Read && access.Action != Write {
		return access
	}
	target := access.Path
	if !filepath.IsAbs(target) {
		target = filepath.Join(r.workspace, target)
	}
	resolved, err := resolveAllowMissing(target)
	if err != nil {
		access.Location = LocationUnknown
		access.ResolutionError = err.Error()
		return access
	}
	access.ResolvedPath = resolved
	if pathWithin(r.workspace, resolved) {
		access.Location = Workspace
	} else {
		access.Location = OutsideWorkspace
	}
	return access
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveAllowMissing resolves every existing symlink in path while retaining
// missing suffixes. Handling dangling symlinks matters for writes: replacing a
// link inside the workspace may otherwise mutate a target outside it.
func resolveAllowMissing(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(abs)
	var suffix []string

	for attempts := 0; attempts < 255; attempts++ {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Clean(appendSuffix(resolved, suffix)), nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		info, lstatErr := os.Lstat(current)
		switch {
		case lstatErr == nil && info.Mode()&os.ModeSymlink != 0:
			target, readErr := os.Readlink(current)
			if readErr != nil {
				return "", readErr
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			current = appendSuffix(filepath.Clean(target), suffix)
			suffix = nil
		case lstatErr != nil && !errors.Is(lstatErr, fs.ErrNotExist):
			return "", lstatErr
		default:
			parent := filepath.Dir(current)
			if parent == current {
				return "", fmt.Errorf("permission: cannot resolve path %q", path)
			}
			suffix = append(suffix, filepath.Base(current))
			current = parent
		}
	}
	return "", fmt.Errorf("permission: too many symlinks resolving %q", path)
}

func appendSuffix(base string, reversed []string) string {
	for i := len(reversed) - 1; i >= 0; i-- {
		base = filepath.Join(base, reversed[i])
	}
	return base
}
