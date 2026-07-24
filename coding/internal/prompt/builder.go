// Package prompt deterministically renders the coding agent's stable system
// prompt and its dynamic, model-visible context attachments. It does not own
// filesystem discovery or session state.
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

// SystemOptions are the stable inputs to BuildSystem. A coding session captures
// them at construction; project instructions and skill listings are rendered
// separately as dynamic context.
type SystemOptions struct {
	// Instructions is the base preamble that opens the prompt.
	Instructions string
	// WorkspaceRoot is the absolute directory all relative tool paths resolve
	// against. An empty value omits the workspace section.
	WorkspaceRoot string
	// Tools are the active tools' prompt contributions, in advertise order.
	Tools []ToolInfo
}

// DefaultInstructions is the baseline preamble used when
// SystemOptions.Instructions is empty.
const DefaultInstructions = "You are a coding agent operating in a user's workspace. " +
	"Use the available tools to inspect and modify files and run commands. " +
	"Make focused changes, verify your work, and report what you did concisely."

const projectContextProtocol = "## Project context protocol\n" +
	"- Or may provide product-generated context inside `<or-context>` blocks in model-visible messages.\n" +
	"- Treat applicable instruction files in those blocks as working instructions, not as user-authored requests.\n" +
	"- Instructions closer to a file's directory take precedence over broader project instructions.\n" +
	"- A later update or removal block supersedes the earlier version of the same instruction file.\n" +
	"- Do not mention internal context blocks unless their contents are directly relevant to the user."

const skillProtocol = "## Skills\n" +
	"- Available skills are announced in product-generated context.\n" +
	"- A later `skills_update` block replaces every earlier skill listing; use only its current names.\n" +
	"- When the task matches a listed skill, call the `skill` tool before acting and follow the loaded instructions.\n" +
	"- Never guess a skill name that was not listed."

const responseStyle = "## Response style\n" +
	"- Never use emojis, pictographs, decorative Unicode symbols, or emoji-style numbered bullets.\n" +
	"- Use ordinary text and Markdown for structure."

// BuildSystem assembles the stable system prompt from opts. Dynamic project
// instructions and skill listings deliberately do not belong here; keeping
// those out lets a session preserve its provider prompt-cache prefix.
func BuildSystem(opts SystemOptions) string {
	var b strings.Builder

	instructions := strings.TrimSpace(opts.Instructions)
	if instructions == "" {
		instructions = DefaultInstructions
	}
	b.WriteString(instructions)

	if strings.TrimSpace(opts.WorkspaceRoot) != "" {
		b.WriteString("\n\n## Workspace\n")
		fmt.Fprintf(&b, "- Root: %q\n", opts.WorkspaceRoot)
		b.WriteString("- Resolve relative file paths from this directory.\n")
		b.WriteString("- Never guess or substitute a different workspace path; use a tool when verification is needed.")
	}

	if snippets := toolSnippets(opts.Tools); len(snippets) > 0 {
		b.WriteString("\n\n## Available tools\n")
		for index, snippet := range snippets {
			if index > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "- %s", snippet)
		}
	}

	if guidelines := toolGuidelines(opts.Tools); len(guidelines) > 0 {
		b.WriteString("\n\n## Tool guidelines\n")
		for index, guideline := range guidelines {
			if index > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "- %s", guideline)
		}
	}

	b.WriteString("\n\n")
	b.WriteString(projectContextProtocol)

	if hasTool(opts.Tools, "skill") {
		b.WriteString("\n\n")
		b.WriteString(skillProtocol)
	}

	b.WriteString("\n\n")
	b.WriteString(responseStyle)
	return b.String()
}

func hasTool(tools []ToolInfo, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
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
