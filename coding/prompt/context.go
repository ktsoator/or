package prompt

import (
	"os"
	"path/filepath"
)

// contextFileNames are the project instruction files recognized in the
// workspace, tried in order; the first that exists in a directory is used.
var contextFileNames = []string{"AGENTS.md", "CLAUDE.md"}

// LoadContextFiles discovers project context files from root up to the
// filesystem root, nearest-last so the most specific instructions read last. In
// each directory the first matching name in contextFileNames is taken. Files
// that cannot be read are skipped.
func LoadContextFiles(root string) []ContextFile {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}

	// Walk from root upward, collecting one file per directory.
	var ancestors []ContextFile
	dir := abs
	for {
		if file, ok := readContextFile(dir); ok {
			ancestors = append(ancestors, file)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Reverse so the outermost (least specific) directory comes first and the
	// workspace root's own file comes last.
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}
	return ancestors
}

// readContextFile returns the first recognized context file in dir, if any.
func readContextFile(dir string) (ContextFile, bool) {
	for _, name := range contextFileNames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return ContextFile{Path: path, Content: string(data)}, true
	}
	return ContextFile{}, false
}
