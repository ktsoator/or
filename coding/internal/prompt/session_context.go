package prompt

import (
	"crypto/sha256"
	"encoding/hex"
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

// maxContextFileChars caps each instruction file. An instruction file is
// projected into every request of the session and sits outside the compactable
// transcript, so one oversized file would permanently occupy context. The cut is
// announced in the rendered block rather than made silently.
const maxContextFileChars = 8_000

// RenderBaseContext renders the session's environment together with its project
// instruction files. It is stable for one context epoch; RenderContextUpdate
// supersedes it when either input changes.
func RenderBaseContext(env Environment, contextFiles []ContextFile) string {
	files := usableContextFiles(contextFiles)
	if env == (Environment{}) && len(files) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<or-context kind=\"base\">\n")
	b.WriteString("This environment and instruction context is provided by Or for the current coding session.\n")
	b.WriteString("Later instruction files are more specific and take precedence.\n")
	renderEnvironment(&b, env)
	renderInstructionFiles(&b, files)
	b.WriteString("</or-context>")
	return b.String()
}

// RenderContextUpdate renders a bounded, self-contained refresh of the
// environment and instruction files. Like a skills update it carries complete
// current state, so this single block supersedes the base context and every
// earlier update without the model replaying a delta history.
func RenderContextUpdate(
	revision string,
	env Environment,
	contextFiles []ContextFile,
) string {
	var b strings.Builder
	fmt.Fprintf(
		&b,
		"<or-context kind=\"context_update\" revision=\"%s\">\n",
		html.EscapeString(revision),
	)
	b.WriteString("The session environment or its instruction files changed. ")
	b.WriteString("This block replaces the earlier base context and every earlier context update.\n")
	renderEnvironment(&b, env)
	renderInstructionFiles(&b, usableContextFiles(contextFiles))
	b.WriteString("</or-context>")
	return b.String()
}

// ContextRevision fingerprints the model-visible environment and instruction
// state. Callers compare revisions to decide whether a refresh is needed, so it
// must depend on exactly what RenderBaseContext shows and nothing else.
func ContextRevision(env Environment, contextFiles []ContextFile) string {
	sum := sha256.Sum256([]byte(RenderBaseContext(env, contextFiles)))
	return hex.EncodeToString(sum[:])
}

func renderEnvironment(b *strings.Builder, env Environment) {
	if env == (Environment{}) {
		return
	}
	b.WriteString("\n<environment>\n")
	writeEnvField(b, "os", env.OS)
	writeEnvField(b, "arch", env.Arch)
	writeEnvField(b, "shell", env.Shell)
	writeEnvField(b, "date", env.Date)
	if env.GitRepo {
		writeEnvField(b, "git-repo", "true")
		writeEnvField(b, "git-branch", env.GitBranch)
	} else {
		writeEnvField(b, "git-repo", "false")
	}
	b.WriteString("</environment>\n")
}

func writeEnvField(b *strings.Builder, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "<%s>%s</%s>\n", name, html.EscapeString(value), name)
}

func renderInstructionFiles(b *strings.Builder, files []ContextFile) {
	if len(files) == 0 {
		return
	}
	b.WriteString("\n<instruction-files>\n")
	for _, file := range files {
		scope := file.Scope
		if scope == "" {
			scope = ScopeProject
		}
		content, truncated := truncateContent(file.Content, maxContextFileChars)
		fmt.Fprintf(
			b,
			"<instruction-file scope=\"%s\" path=\"%s\"%s>\n%s\n</instruction-file>\n",
			html.EscapeString(string(scope)),
			html.EscapeString(file.Path),
			truncatedAttr(truncated),
			strings.TrimRight(content, "\n"),
		)
	}
	b.WriteString("</instruction-files>\n")
}

func truncatedAttr(truncated bool) string {
	if !truncated {
		return ""
	}
	return " truncated=\"true\""
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

// truncateContent caps an instruction file at n runes, keeping the head and
// appending a notice the model can act on. It reports whether it cut.
func truncateContent(s string, n int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= n {
		return s, false
	}
	dropped := len(runes) - n
	return fmt.Sprintf(
		"%s\n\n[truncated: %d more character(s) not shown; read the file directly if you need the rest]",
		strings.TrimRight(string(runes[:n]), "\n"),
		dropped,
	), true
}
