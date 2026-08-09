package prompt

import (
	"os"
	"path/filepath"
)

const (
	contextFileName      = "AGENTS.md"
	localContextFileName = "AGENTS.local.md"
)

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
		if file, ok := readContextFile(dir, contextFileName, ScopeUser); ok {
			files = append(files, file)
		}
	}

	// Walk from the workspace root upward, then reverse, so the outermost
	// (least specific) ancestor comes first and the workspace root's own file
	// comes last.
	var ancestors []ContextFile
	dir := abs
	for {
		if file, ok := readContextFile(dir, contextFileName, ScopeProject); ok {
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

	if file, ok := readContextFile(abs, localContextFileName, ScopeLocal); ok {
		files = append(files, file)
	}
	return files
}

func readContextFile(dir, name string, scope ContextScope) (ContextFile, bool) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return ContextFile{}, false
	}
	return ContextFile{Path: path, Content: string(data), Scope: scope}, true
}
