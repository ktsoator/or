// Package prompt assembles the coding agent's system prompt. The prompt is
// built from whichever tools are active — each tool contributes its own one-line
// description and guideline bullets — plus any project context files. This keeps
// the prompt in sync with the tool set instead of maintained as one central
// block.
package prompt

import (
	"fmt"
	"strings"
)

// ToolInfo is a tool's contribution to the system prompt.
type ToolInfo struct {
	// Name is the tool's advertised name.
	Name string
	// Snippet is a one-line description for the Available tools section. An empty
	// snippet omits the tool from that section.
	Snippet string
	// Guidelines are bullets appended to the Guidelines section.
	Guidelines []string
}

// ContextFile is a project document (for example AGENTS.md) included verbatim so
// the model follows repository-specific instructions.
type ContextFile struct {
	Path    string
	Content string
}

// Options are the inputs to Build.
type Options struct {
	// Instructions is the base preamble that opens the prompt.
	Instructions string
	// Tools are the active tools' prompt contributions, in advertise order.
	Tools []ToolInfo
	// ContextFiles are project context documents included after the tool
	// sections.
	ContextFiles []ContextFile
}

// DefaultInstructions is the baseline preamble used when Options.Instructions is
// empty.
const DefaultInstructions = "You are a coding agent operating in a user's workspace. " +
	"Use the available tools to inspect and modify files and run commands. " +
	"Make focused changes, verify your work, and report what you did concisely."

const responseStyle = "## Response style\n" +
	"- Never use emojis, pictographs, decorative Unicode symbols, or emoji-style numbered bullets.\n" +
	"- Use ordinary text and Markdown for structure."

// Build assembles the system prompt from opts. Sections with no content are
// omitted, so a minimal configuration yields a short prompt.
func Build(opts Options) string {
	var b strings.Builder

	instructions := opts.Instructions
	if strings.TrimSpace(instructions) == "" {
		instructions = DefaultInstructions
	}
	b.WriteString(instructions)
	b.WriteString("\n\n")
	b.WriteString(responseStyle)

	if snippets := toolSnippets(opts.Tools); len(snippets) > 0 {
		b.WriteString("\n\n## Available tools\n")
		for _, s := range snippets {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}

	if guidelines := toolGuidelines(opts.Tools); len(guidelines) > 0 {
		b.WriteString("\n## Guidelines\n")
		for _, g := range guidelines {
			fmt.Fprintf(&b, "- %s\n", g)
		}
	}

	for _, file := range opts.ContextFiles {
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		fmt.Fprintf(&b, "\n## Project context: %s\n\n%s\n", file.Path, strings.TrimRight(file.Content, "\n"))
	}

	return b.String()
}

// toolSnippets collects the non-empty snippets in order.
func toolSnippets(tools []ToolInfo) []string {
	var out []string
	for _, t := range tools {
		if strings.TrimSpace(t.Snippet) != "" {
			out = append(out, t.Snippet)
		}
	}
	return out
}

// toolGuidelines collects guideline bullets across tools, dropping duplicates
// while preserving first-seen order.
func toolGuidelines(tools []ToolInfo) []string {
	seen := make(map[string]bool)
	var out []string
	for _, t := range tools {
		for _, g := range t.Guidelines {
			g = strings.TrimSpace(g)
			if g == "" || seen[g] {
				continue
			}
			seen[g] = true
			out = append(out, g)
		}
	}
	return out
}
