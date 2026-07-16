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
}

// Name returns the tool's advertised name.
func (t Tool) Name() string { return t.Definition.Name }

// CodingTools returns the default v1 tool set rooted at the given workspace
// directory and backed by ops. Pass LocalOps{} for the local filesystem and
// shell.
func CodingTools(root string, ops Ops) []Tool {
	return []Tool{
		Read(root, ops),
		Bash(root, ops),
		Edit(root, ops),
		Write(root, ops),
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

// resolve turns a possibly-relative tool argument path into an absolute path
// rooted at the workspace. Absolute inputs are returned cleaned, unchanged in
// meaning.
func resolve(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}
