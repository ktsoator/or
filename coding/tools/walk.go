package tools

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// skipDirs are directory names pruned from workspace traversal by grep and glob.
// They are the common vendored and version-control directories that bloat search
// output and are rarely what the model is looking for. This is a pragmatic
// dependency-free stand-in for full .gitignore handling.
var skipDirs = map[string]bool{
	".git":          true,
	".hg":           true,
	".svn":          true,
	"node_modules":  true,
	"vendor":        true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	".mypy_cache":   true,
	".pytest_cache": true,
	"dist":          true,
	"build":         true,
	"target":        true,
	".idea":         true,
	".vscode":       true,
	".next":         true,
	".cache":        true,
}

// walkedFile is one file found during a workspace walk.
type walkedFile struct {
	// rel is the file's path relative to the walk root.
	rel string
	// info is the file's metadata, for size and modification time.
	info os.FileInfo
}

// walkFiles returns every non-skipped regular file under root, with paths
// relative to root, via the FileOps seam. Directories named in skipDirs are
// pruned. The walk stops early and returns ctx.Err() if the context is
// cancelled.
func walkFiles(ctx context.Context, ops FileOps, root string) ([]walkedFile, error) {
	var out []walkedFile
	var walk func(dir, rel string) error
	walk = func(dir, rel string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := ops.ReadDir(ctx, dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := entry.Name()
			childRel := name
			if rel != "" {
				childRel = rel + "/" + name
			}
			if entry.IsDir() {
				if skipDirs[name] {
					continue
				}
				if err := walk(filepath.Join(dir, name), childRel); err != nil {
					return err
				}
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue // entry vanished between listing and stat; skip it
			}
			out = append(out, walkedFile{rel: childRel, info: info})
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	return out, nil
}

// globToRegexp translates a glob pattern into an anchored regular expression
// matched against a forward-slash relative path. It supports `*` (any run within
// a path segment), `?` (one non-separator character), and `**` (any number of
// segments); a leading `**/` also matches zero directories.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			b.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
