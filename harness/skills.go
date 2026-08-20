package harness

import (
	"context"
	"fmt"
	"strings"
)

// Skill is a named set of instructions the harness can inject as a user message. Unlike
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

// Skill invokes a registered skill by name: it injects the skill's instructions
// (plus any additional instructions) as a new user message and runs it like Prompt.
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

// SetSkills replaces the registered skills. Changes apply from the next run.
func (h *Harness) SetSkills(skills []Skill) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	h.skills = append([]Skill(nil), skills...)
}

// Skills returns a copy of the registered skills.
func (h *Harness) Skills() []Skill { return h.skillsSnapshot() }

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

// formatSkillInvocation renders a skill as the user message that invokes it.
func formatSkillInvocation(skill Skill, additionalInstructions string) string {
	block := fmt.Sprintf("<skill name=%q>\n%s\n</skill>", skill.Name, skill.Content)
	if additionalInstructions != "" {
		return block + "\n\n" + additionalInstructions
	}
	return block
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
