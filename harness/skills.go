package harness

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Skill is a named set of instructions the harness can inject as a turn. Unlike
// the reference implementation, a Skill is in-memory content rather than a file:
// loading from disk is left to the caller.
type Skill struct {
	// Name is the stable identifier used for lookup and model-visible listings.
	Name string
	// Description is a short, model-visible note on when to use the skill.
	Description string
	// Content is the full instructions injected when the skill is invoked.
	Content string
	// DisableModelInvocation hides the skill from FormatSkillsForSystemPrompt
	// while still allowing explicit invocation via Skill.
	DisableModelInvocation bool
}

// PromptTemplate is a named, parameterized prompt invoked with PromptFromTemplate.
// In Content, $1..$N expand to positional arguments and $ARGUMENTS (or $@) to all
// arguments joined by spaces.
type PromptTemplate struct {
	// Name is the stable identifier used for lookup.
	Name string
	// Description is an optional note for command lists or autocomplete.
	Description string
	// Content is the template body with argument placeholders.
	Content string
}

// Skill invokes a registered skill by name: it injects the skill's instructions
// (plus any additional instructions) as a new user turn and runs it like Prompt.
// It returns an error if the skill is unknown, or ErrBusy if a run is already in
// progress.
func (h *Harness) Skill(ctx context.Context, name string, additionalInstructions ...string) error {
	h.cfgMu.Lock()
	skill, ok := findSkill(h.skills, name)
	h.cfgMu.Unlock()
	if !ok {
		return fmt.Errorf("harness: unknown skill: %s", name)
	}
	return h.Prompt(ctx, formatSkillInvocation(skill, strings.Join(additionalInstructions, "\n\n")))
}

// PromptFromTemplate invokes a registered prompt template by name, substituting
// args into its placeholders, and runs the result like Prompt. It returns an
// error if the template is unknown, or ErrBusy if a run is already in progress.
func (h *Harness) PromptFromTemplate(ctx context.Context, name string, args ...string) error {
	h.cfgMu.Lock()
	template, ok := findTemplate(h.templates, name)
	h.cfgMu.Unlock()
	if !ok {
		return fmt.Errorf("harness: unknown prompt template: %s", name)
	}
	return h.Prompt(ctx, substituteArgs(template.Content, args))
}

// SetSkills replaces the registered skills. Changes apply from the next run.
func (h *Harness) SetSkills(skills []Skill) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	h.skills = append([]Skill(nil), skills...)
}

// SetPromptTemplates replaces the registered prompt templates.
func (h *Harness) SetPromptTemplates(templates []PromptTemplate) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	h.templates = append([]PromptTemplate(nil), templates...)
}

// Skills returns a copy of the registered skills.
func (h *Harness) Skills() []Skill { return h.skillsSnapshot() }

// PromptTemplates returns a copy of the registered prompt templates.
func (h *Harness) PromptTemplates() []PromptTemplate {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return append([]PromptTemplate(nil), h.templates...)
}

func (h *Harness) skillsSnapshot() []Skill {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return append([]Skill(nil), h.skills...)
}

func findSkill(skills []Skill, name string) (Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return Skill{}, false
}

func findTemplate(templates []PromptTemplate, name string) (PromptTemplate, bool) {
	for _, template := range templates {
		if template.Name == name {
			return template, true
		}
	}
	return PromptTemplate{}, false
}

// formatSkillInvocation renders a skill as the user turn that invokes it.
func formatSkillInvocation(skill Skill, additionalInstructions string) string {
	block := fmt.Sprintf("<skill name=%q>\n%s\n</skill>", skill.Name, skill.Content)
	if additionalInstructions != "" {
		return block + "\n\n" + additionalInstructions
	}
	return block
}

var positionalArg = regexp.MustCompile(`\$(\d+)`)

// substituteArgs expands $1..$N to positional arguments and $ARGUMENTS / $@ to
// all arguments joined by spaces.
func substituteArgs(content string, args []string) string {
	result := positionalArg.ReplaceAllStringFunc(content, func(match string) string {
		index := 0
		fmt.Sscanf(match, "$%d", &index)
		if index >= 1 && index <= len(args) {
			return args[index-1]
		}
		return ""
	})
	all := strings.Join(args, " ")
	result = strings.ReplaceAll(result, "$ARGUMENTS", all)
	result = strings.ReplaceAll(result, "$@", all)
	return result
}

// FormatSkillsForSystemPrompt renders the model-invocable skills as a block to
// include in a system prompt, so the model can choose to use them. Skills with
// DisableModelInvocation set are omitted; it returns "" when none are visible.
func FormatSkillsForSystemPrompt(skills []Skill) string {
	var visible []Skill
	for _, skill := range skills {
		if !skill.DisableModelInvocation {
			visible = append(visible, skill)
		}
	}
	if len(visible) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The following skills provide specialized instructions for specific tasks. ")
	b.WriteString("Use one when the task matches its description.\n\n<available_skills>\n")
	for _, skill := range visible {
		fmt.Fprintf(&b, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n  </skill>\n",
			escapeXML(skill.Name), escapeXML(skill.Description))
	}
	b.WriteString("</available_skills>")
	return b.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
