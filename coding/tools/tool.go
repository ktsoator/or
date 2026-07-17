package tools

import (
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// Tool is a coding-agent tool: the executable agent.AgentTool the model calls,
// plus the metadata the tool contributes to the system prompt. Keeping prompt
// contribution on the tool itself means the system prompt is assembled from
// whichever tools are active, rather than maintained as one central block.
type Tool struct {
	agent.AgentTool

	// PromptSnippet is a one-line description for the "Available tools" section
	// of the system prompt. An empty snippet omits the tool from that section.
	PromptSnippet string
	// Guidelines are bullet points appended to the system prompt's guidelines
	// section while this tool is active.
	Guidelines []string
	// ReadOnly reports whether the tool leaves the workspace unchanged. The
	// permission layer uses it to decide what needs confirmation.
	ReadOnly bool
	// ReadOnlyFor optionally decides read-only status per call from the validated
	// arguments, for a tool like bash whose effect depends on its input. When set
	// it overrides ReadOnly for that call; nil falls back to the static ReadOnly.
	// A conservative implementation returns false whenever it is unsure.
	ReadOnlyFor func(args map[string]any) bool
}

// IsReadOnly reports whether a specific call is read-only, using the per-call
// classifier when the tool provides one and the static flag otherwise.
func (t Tool) IsReadOnly(args map[string]any) bool {
	if t.ReadOnlyFor != nil {
		return t.ReadOnlyFor(args)
	}
	return t.ReadOnly
}

// Name returns the tool's advertised name.
func (t Tool) Name() string { return t.Definition.Name }

// CodingTools returns the default v1 tool set rooted at the given workspace
// directory and backed by ops. One file-state store is shared by Read, Edit,
// and Write for this tool-set lifetime. Pass LocalOps{} for the local filesystem
// and shell.
func CodingTools(root string, ops Ops) []Tool {
	files := NewFileStateStore()
	return []Tool{
		Read(root, ops, files),
		Grep(root, ops),
		Glob(root, ops),
		LS(root, ops),
		Edit(root, ops, files),
		Write(root, ops, files),
		Bash(root, ops),
	}
}

// AgentTools extracts the executable agent.AgentTool from each Tool, for handing
// to the agent loop.
func AgentTools(tools []Tool) []agent.AgentTool {
	out := make([]agent.AgentTool, len(tools))
	for i, t := range tools {
		out[i] = t.AgentTool
	}
	return out
}

// textResult builds a ToolResult carrying a single text block.
func textResult(text string) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
	}
}

// resultWith builds a ToolResult whose model-facing text is derived from a
// structured Details value. The text is what the model reads; Details is the
// source of truth product shells render.
func resultWith(text string, details any) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Details: details,
	}
}

// mutationFailure builds a failed edit/write result carrying both the text
// detail the model reads and a structured MutationFailure product shells render.
func mutationFailure(path, reason, detail string) agent.ToolResult {
	return resultWith(detail, MutationFailure{Path: path, Reason: reason, Detail: detail})
}

// resolve turns a possibly-relative tool argument path into an absolute path
// rooted at the workspace. Absolute inputs are returned cleaned, unchanged in
// meaning.
func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}
