package prompttemplate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

var promptsDir = []string{".or", "prompts"}

type frontmatter struct {
	Description      string `yaml:"description"`
	DescriptionEN    string `yaml:"description-en"`
	DescriptionZhCN  string `yaml:"description-zh-CN"`
	ArgumentHint     string `yaml:"argument-hint"`
	ArgumentHintEN   string `yaml:"argument-hint-en"`
	ArgumentHintZhCN string `yaml:"argument-hint-zh-CN"`
}

type LoadOptions struct {
	UserDir    string
	ProjectDir string
}

// Roots returns the user-level and workspace-level prompt directories.
func Roots(workspace string) (userDir, projectDir string) {
	if home, err := os.UserHomeDir(); err == nil {
		userDir = filepath.Join(append([]string{home}, promptsDir...)...)
	}
	if ws := strings.TrimSpace(workspace); ws != "" {
		projectDir = filepath.Join(append([]string{ws}, promptsDir...)...)
	}
	return userDir, projectDir
}

func LoadFor(workspace string) (*Registry, []Diagnostic) {
	userDir, projectDir := Roots(workspace)
	return Load(LoadOptions{UserDir: userDir, ProjectDir: projectDir})
}

// Load discovers Markdown files directly inside the configured roots. Project
// templates replace user templates with the same filename-derived name.
func Load(opts LoadOptions) (*Registry, []Diagnostic) {
	byName := map[string]Template{}
	var diagnostics []Diagnostic
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
		loaded, foundDiagnostics := loadRoot(root.dir, root.source)
		diagnostics = append(diagnostics, foundDiagnostics...)
		for _, template := range loaded {
			byName[template.Name] = template
		}
	}
	return newRegistry(byName), diagnostics
}

func loadRoot(root string, source Source) ([]Template, []Diagnostic) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Diagnostic{{Path: root, Message: fmt.Sprintf("read prompts root: %v", err)}}
	}

	var templates []Template
	var diagnostics []Diagnostic
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		template, diagnostic, ok := loadFile(path, source)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
		if ok {
			templates = append(templates, template)
		}
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].Name < templates[j].Name })
	return templates, diagnostics
}

func loadFile(path string, source Source) (Template, *Diagnostic, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Template{}, &Diagnostic{Path: path, Message: fmt.Sprintf("read template: %v", err)}, false
	}
	fm, body, err := parseTemplate(string(raw))
	if err != nil {
		return Template{}, &Diagnostic{Path: path, Message: err.Error()}, false
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if !validName(name) {
		return Template{}, &Diagnostic{Path: path, Message: fmt.Sprintf("invalid template name %q", name)}, false
	}
	descriptions := localizedValues(fm.DescriptionEN, fm.DescriptionZhCN)
	description := strings.TrimSpace(fm.Description)
	if description == "" {
		description = localizedFallback(descriptions)
	}
	if description == "" {
		description = firstLineDescription(body)
	}
	argumentHints := localizedValues(fm.ArgumentHintEN, fm.ArgumentHintZhCN)
	argumentHint := strings.TrimSpace(fm.ArgumentHint)
	if argumentHint == "" {
		argumentHint = localizedFallback(argumentHints)
	}
	return Template{
		Name:          name,
		Description:   description,
		Descriptions:  descriptions,
		ArgumentHint:  argumentHint,
		ArgumentHints: argumentHints,
		Content:       body,
		Path:          path,
		Source:        source,
	}, nil, true
}

func localizedValues(english, chinese string) map[string]string {
	values := map[string]string{}
	if value := strings.TrimSpace(english); value != "" {
		values["en"] = value
	}
	if value := strings.TrimSpace(chinese); value != "" {
		values["zh-CN"] = value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func localizedFallback(values map[string]string) string {
	if value := values["en"]; value != "" {
		return value
	}
	return values["zh-CN"]
}

func parseTemplate(raw string) (frontmatter, string, error) {
	raw = strings.TrimPrefix(raw, "\ufeff")
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(raw, "---\r\n")
	}
	if !ok {
		return frontmatter{}, raw, nil
	}
	start, next := closingFence(rest)
	if start < 0 {
		return frontmatter{}, "", fmt.Errorf("unterminated YAML frontmatter (missing closing '---' line)")
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:start]), &fm); err != nil {
		return frontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, strings.TrimLeft(rest[next:], "\r\n"), nil
}

func closingFence(rest string) (start, next int) {
	for offset := 0; offset < len(rest); {
		nl := strings.IndexByte(rest[offset:], '\n')
		lineEnd := len(rest)
		line := rest[offset:]
		if nl >= 0 {
			line = rest[offset : offset+nl]
			lineEnd = offset + nl + 1
		}
		if strings.TrimRight(line, "\r") == "---" {
			return offset, lineEnd
		}
		if nl < 0 {
			break
		}
		offset = lineEnd
	}
	return -1, -1
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range name {
		if char == ':' || char == '/' || char == '\\' || char == utf8.RuneError ||
			char == ' ' || char == '\t' || char == '\r' || char == '\n' {
			return false
		}
	}
	return true
}

func firstLineDescription(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 60 {
			return string(runes[:60]) + "..."
		}
		return line
	}
	return ""
}
