package prompt

import (
	"os"
	"path/filepath"
)

// contextFileNames are the project instruction files recognized in the
// workspace, tried in order; the first that exists in a directory is used.
var contextFileNames = []string{"AGENTS.md", "CLAUDE.md"}

// localContextFileNames are the workspace-root instruction files meant to stay
// out of version control. They are the most specific layer, so they load last.
var localContextFileNames = []string{"AGENTS.local.md", "CLAUDE.local.md"}

// userContextDir is the user-level instruction directory, relative to the home
// directory. It matches the skills root so both layers live under one dot dir.
var userContextDir = []string{".or"}

// LoadContextFiles discovers every instruction file that applies to a workspace,
// broadest first so the most specific instructions read last:
//
//  1. the user-level file under ~/.or, applied to every workspace;
//  2. one project file per directory from the filesystem root down to the
//     workspace root;
//  3. the workspace root's local, uncommitted file.
//
// Files that cannot be read are skipped.
func LoadContextFiles(root string) []ContextFile {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}

	var files []ContextFile
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(append([]string{home}, userContextDir...)...)
		if file, ok := readContextFile(dir, contextFileNames, ScopeUser); ok {
			files = append(files, file)
		}
	}

	// Walk from the workspace root upward, then reverse, so the outermost
	// (least specific) ancestor comes first and the workspace root's own file
	// comes last.
	var ancestors []ContextFile
	dir := abs
	for {
		if file, ok := readContextFile(dir, contextFileNames, ScopeProject); ok {
			ancestors = append(ancestors, file)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}
	files = append(files, ancestors...)

	if file, ok := readContextFile(abs, localContextFileNames, ScopeLocal); ok {
		files = append(files, file)
	}
	return files
}

// readContextFile returns the first of names that exists in dir, if any.
func readContextFile(dir string, names []string, scope ContextScope) (ContextFile, bool) {
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return ContextFile{Path: path, Content: string(data), Scope: scope}, true
	}
	return ContextFile{}, false
}
