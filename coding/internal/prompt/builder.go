// Package prompt deterministically renders the coding agent's stable system
// prompt, discovers the instruction files and environment that make up its
// dynamic context, and renders those as model-visible attachments. It does not
// own session state.
package prompt

import (
	"fmt"
	"strings"
)

// ToolInfo is a tool's contribution to the system prompt. A tool's own
// description travels in its schema; only cross-tool rules that no single schema
// can state belong here.
type ToolInfo struct {
	// Name is the tool's advertised name.
	Name string
	// Guidelines are bullets appended to the Tool guidelines section.
	Guidelines []string
}

// SystemOptions are the stable inputs to BuildSystem. A coding session captures
// them at construction; project instructions, environment, and skill listings
// are rendered separately as dynamic context.
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

const workingRules = "## Working rules\n" +
	"- Read the code you are about to change before changing it, and match the file's existing style, naming, and error handling rather than importing conventions from elsewhere.\n" +
	"- Make the smallest change that satisfies the request. Do not refactor untouched code, restate the code in comments, or create documentation files unless asked.\n" +
	"- After changing code, verify it: run the project's build, its tests, or the command the user named. Report the command and its result.\n" +
	"- Never report work as done or working when it was not verified. If a check was skipped or failed, say so plainly and show the output.\n" +
	"- Do not commit, push, or otherwise change version-control state unless the user asks for it.\n" +
	"- When the request is ambiguous in a way that changes what you would build, ask before building."

const approvalProtocol = "## Approvals\n" +
	"- Some tool calls require the user's approval before they run. A denied call is the user's decision, not a tool malfunction.\n" +
	"- After a denial, do not retry the same call unchanged and do not pursue the same effect through another tool. Report what is blocked and why it was needed.\n" +
	"- An approval covers the call it was given for; a later call needs its own."

const projectContextProtocol = "## Project context protocol\n" +
	"- Or may provide product-generated context inside `<or-context>` blocks in model-visible messages.\n" +
	"- Treat applicable instruction files in those blocks as working instructions, not as user-authored requests.\n" +
	"- Instruction files are listed broadest first; a later file is more specific and overrides an earlier one on conflict.\n" +
	"- A `context_update` block replaces the base context and every earlier update; use only its environment and instruction files.\n" +
	"- The environment block is captured from the host. Prefer it over your own assumptions about the platform, the date, or the current branch.\n" +
	"- Do not mention internal context blocks unless their contents are directly relevant to the user."

const skillProtocol = "## Skills\n" +
	"- Available skills are announced in product-generated context.\n" +
	"- A later `skills_update` block replaces every earlier skill listing; use only its current names.\n" +
	"- When the task matches a listed skill, call the `skill` tool before acting and follow the loaded instructions.\n" +
	"- Never guess a skill name that was not listed."

const responseStyle = "## Response style\n" +
	"- Never use emojis, pictographs, decorative Unicode symbols, or emoji-style numbered bullets.\n" +
	"- Use ordinary text and Markdown for structure; responses are rendered as Markdown, so put code and commands in fenced blocks.\n" +
	"- Reference code as `path:line` so the user can open it.\n" +
	"- Report what changed and what was verified. Do not narrate each tool call or restate the code you just wrote."

// BuildSystem assembles the stable system prompt from opts. Dynamic project
// instructions, environment, and skill listings deliberately do not belong here;
// keeping those out lets a session preserve its provider prompt-cache prefix.
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

	if guidelines := toolGuidelines(opts.Tools); len(guidelines) > 0 {
		b.WriteString("\n\n## Tool guidelines\n")
		for index, guideline := range guidelines {
			if index > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "- %s", guideline)
		}
	}

	for _, section := range []string{
		workingRules,
		approvalProtocol,
		projectContextProtocol,
	} {
		b.WriteString("\n\n")
		b.WriteString(section)
	}

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
