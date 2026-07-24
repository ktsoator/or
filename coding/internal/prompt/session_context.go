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

// SkillsDelta describes the discovery changes included in a skills-update
// attachment. Current state is rendered separately in the same attachment so a
// model never has to replay an unbounded delta history.
type SkillsDelta struct {
	Added   []SkillInfo
	Updated []SkillInfo
	Removed []string
}

// maxSkillDescChars caps each skill description in the discovery listing. The
// skill tool loads complete instructions on demand.
const maxSkillDescChars = 240

// RenderBaseContext renders project instruction files independently from skill
// discovery metadata. It is stable for one context epoch.
func RenderBaseContext(contextFiles []ContextFile) string {
	files := usableContextFiles(contextFiles)
	if len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<or-context kind=\"base\">\n")
	b.WriteString("This instruction context is provided by Or for the current coding session.\n")
	b.WriteString("Later and more specific instruction files take precedence.\n")
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
	b.WriteString("</or-context>")
	return b.String()
}

// RenderSkillListing renders the initial discovery snapshot. An empty skill set
// needs no attachment because the stable skill tool already handles unknown
// names safely.
func RenderSkillListing(revision string, skills []SkillInfo) string {
	skillList := usableSkills(skills)
	if len(skillList) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(
		&b,
		"<or-context kind=\"skill_listing\" revision=\"%s\">\n",
		html.EscapeString(revision),
	)
	b.WriteString("These are the skills currently available through the stable `skill` tool.\n")
	renderAvailableSkills(&b, skillList)
	b.WriteString("</or-context>")
	return b.String()
}

// RenderSkillsUpdate renders a bounded, self-contained refresh. It identifies
// what changed but also includes the complete current listing; this single block
// supersedes all earlier skill listings and updates.
func RenderSkillsUpdate(
	revision string,
	current []SkillInfo,
	delta SkillsDelta,
) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"<or-context kind=\"skills_update\" revision=\"%s\">\n",
		html.EscapeString(revision),
	)
	b.WriteString("The available skill set changed. This block replaces every earlier skill listing and skills update.\n")
	b.WriteString("<changes>\n")
	renderSkillChange(&b, "added", delta.Added)
	renderSkillChange(&b, "updated", delta.Updated)
	removed := usableRemovedSkills(delta.Removed)
	if len(removed) > 0 {
		b.WriteString("<removed>\n")
		for _, name := range removed {
			fmt.Fprintf(&b, "<name>%s</name>\n", html.EscapeString(name))
		}
		b.WriteString("</removed>\n")
	}
	if len(usableSkills(delta.Added)) == 0 &&
		len(usableSkills(delta.Updated)) == 0 &&
		len(removed) == 0 {
		b.WriteString("<none />\n")
	}
	b.WriteString("</changes>\n")
	renderAvailableSkills(&b, usableSkills(current))
	b.WriteString("</or-context>")
	return b.String()
}

func renderSkillChange(b *strings.Builder, tag string, skills []SkillInfo) {
	list := usableSkills(skills)
	if len(list) == 0 {
		return
	}
	fmt.Fprintf(b, "<%s>\n", tag)
	renderSkillEntries(b, list)
	fmt.Fprintf(b, "</%s>\n", tag)
}

func renderAvailableSkills(b *strings.Builder, skills []SkillInfo) {
	if len(skills) == 0 {
		b.WriteString("<available-skills none=\"true\" />\n")
		return
	}
	b.WriteString("<available-skills>\n")
	renderSkillEntries(b, skills)
	b.WriteString("</available-skills>\n")
}

func renderSkillEntries(b *strings.Builder, skills []SkillInfo) {
	for _, skill := range skills {
		b.WriteString("<skill>\n")
		fmt.Fprintf(b, "<name>%s</name>\n", html.EscapeString(skill.Name))
		fmt.Fprintf(
			b,
			"<description>%s</description>\n",
			html.EscapeString(truncateChars(skill.Description, maxSkillDescChars)),
		)
		b.WriteString("</skill>\n")
	}
}

func usableRemovedSkills(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	sort.Strings(result)
	return result
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
