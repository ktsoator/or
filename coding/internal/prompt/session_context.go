package prompt

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// ContextScope identifies where an instruction file was discovered. Scope is
// model-visible so precedence remains explicit when different layers use the
// same filename.
type ContextScope string

const (
	ScopeUser    ContextScope = "user"
	ScopeProject ContextScope = "project"
	ScopeLocal   ContextScope = "local"
	ScopeNested  ContextScope = "nested"
)

// ContextFile is one instruction document rendered into model-visible context.
// Files must arrive in precedence order, broadest first and most specific last.
type ContextFile struct {
	Path    string
	Content string
	Scope   ContextScope
}

// SkillInfo is the discovery metadata advertised before a skill is loaded.
// Complete instructions remain behind the skill tool.
type SkillInfo struct {
	Name        string
	Description string
}

// SessionContextOptions are the dynamic inputs to RenderSessionContext.
type SessionContextOptions struct {
	ContextFiles []ContextFile
	Skills       []SkillInfo
}

// maxSkillDescChars caps each skill description in the discovery listing. The
// skill tool loads complete instructions on demand.
const maxSkillDescChars = 240

// RenderSessionContext renders the initial, model-visible session context. It
// returns an empty string when there are no usable instruction files or skills.
func RenderSessionContext(opts SessionContextOptions) string {
	files := usableContextFiles(opts.ContextFiles)
	skillList := usableSkills(opts.Skills)
	if len(files) == 0 && len(skillList) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<or-context kind=\"session\">\n")
	b.WriteString("This context is provided by Or for the current coding session.\n")
	b.WriteString("Later and more specific instruction files take precedence.\n")

	if len(files) > 0 {
		b.WriteString("\n<instruction-files>\n")
		for _, file := range files {
			scope := file.Scope
			if scope == "" {
				scope = ScopeProject
			}
			fmt.Fprintf(
				&b,
				"<instruction-file scope=\"%s\" path=\"%s\">\n%s\n</instruction-file>\n",
				html.EscapeString(string(scope)),
				html.EscapeString(file.Path),
				strings.TrimRight(file.Content, "\n"),
			)
		}
		b.WriteString("</instruction-files>\n")
	}

	if len(skillList) > 0 {
		b.WriteString("\n<available-skills>\n")
		for _, skill := range skillList {
			b.WriteString("<skill>\n")
			fmt.Fprintf(&b, "<name>%s</name>\n", html.EscapeString(skill.Name))
			fmt.Fprintf(
				&b,
				"<description>%s</description>\n",
				html.EscapeString(truncateChars(skill.Description, maxSkillDescChars)),
			)
			b.WriteString("</skill>\n")
		}
		b.WriteString("</available-skills>\n")
	}

	b.WriteString("</or-context>")
	return b.String()
}

func usableContextFiles(files []ContextFile) []ContextFile {
	result := make([]ContextFile, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		result = append(result, file)
	}
	return result
}

func usableSkills(skills []SkillInfo) []SkillInfo {
	result := make([]SkillInfo, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		result = append(result, SkillInfo{
			Name:        name,
			Description: strings.TrimSpace(skill.Description),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// truncateChars shortens s to at most n runes, appending an ellipsis when cut.
func truncateChars(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
