package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// skillFile is the required filename inside each skill directory.
const skillFile = "SKILL.md"

// LoadOptions names the two roots skills are discovered from. Either may be
// empty to skip that root. Both should be absolute paths to a skills directory,
// e.g. ~/.or/skills and <workspace>/.or/skills.
type LoadOptions struct {
	// UserDir is the user-level skills root, applied to every workspace.
	UserDir string
	// ProjectDir is the workspace-scoped skills root. A skill here overrides a
	// user skill of the same name.
	ProjectDir string
}

// Load discovers skills under the configured roots and returns a Registry.
// Project skills override user skills of the same name. Malformed or misnamed
// skills are skipped and reported as diagnostics rather than failing the load,
// so one bad skill does not hide the rest.
func Load(opts LoadOptions) (*Registry, []Diagnostic) {
	byName := map[string]Skill{}
	var diags []Diagnostic

	// User first, then project, so project entries overwrite on name collision.
	for _, root := range []struct {
		dir    string
		source Source
	}{
		{opts.UserDir, SourceUser},
		{opts.ProjectDir, SourceProject},
	} {
		if strings.TrimSpace(root.dir) == "" {
			continue
		}
		loaded, d := loadRoot(root.dir, root.source)
		diags = append(diags, d...)
		for _, s := range loaded {
			byName[s.Name] = s
		}
	}

	return newRegistry(byName), diags
}

// loadRoot scans one skills root: each immediate subdirectory is a candidate
// skill whose SKILL.md is parsed and validated.
func loadRoot(root string, source Source) ([]Skill, []Diagnostic) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // an absent root is normal, not an error
		}
		return nil, []Diagnostic{{Path: root, Message: fmt.Sprintf("read skills root: %v", err)}}
	}

	var skills []Skill
	var diags []Diagnostic
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip dotfiles/dirs
		}
		dir := filepath.Join(root, name)
		// Follow symlinked directories, matching directory-only skill layout.
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		s, diag, ok := loadSkill(dir, name, source)
		if diag != nil {
			diags = append(diags, *diag)
		}
		if ok {
			skills = append(skills, s)
		}
	}
	return skills, diags
}

// loadSkill reads and validates a single skill directory. It returns ok=false
// when the directory is not a skill (no SKILL.md) or is invalid, with a
// diagnostic in the invalid case.
func loadSkill(dir, dirName string, source Source) (Skill, *Diagnostic, bool) {
	path := filepath.Join(dir, skillFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Skill{}, nil, false // not a skill directory; silently ignore
		}
		return Skill{}, &Diagnostic{Path: path, Message: fmt.Sprintf("read %s: %v", skillFile, err)}, false
	}

	fm, body, err := parseSKILL(string(raw))
	if err != nil {
		return Skill{}, &Diagnostic{Path: path, Message: err.Error()}, false
	}
	if strings.TrimSpace(fm.Name) == "" {
		return Skill{}, &Diagnostic{Path: path, Message: "frontmatter is missing required field: name"}, false
	}
	if fm.Name != dirName {
		return Skill{}, &Diagnostic{Path: path, Message: fmt.Sprintf("frontmatter name %q must match directory name %q", fm.Name, dirName)}, false
	}
	if strings.TrimSpace(fm.Description) == "" {
		return Skill{}, &Diagnostic{Path: path, Message: "frontmatter is missing required field: description"}, false
	}

	return Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Content:     body,
		Dir:         dir,
		Path:        path,
		Source:      source,
	}, nil, true
}
