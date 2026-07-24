package tools

import (
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

// Tool is a coding-agent tool: the executable agent.AgentTool the model calls,
// plus the metadata the tool contributes to the system prompt. Keeping prompt
// contribution on the tool itself means the system prompt is assembled from
// whichever tools are active, rather than maintained as one central block.
type Tool struct {
	agent.AgentTool

	// Guidelines are bullet points appended to the system prompt's guidelines
	// section while this tool is active. A tool's own description travels in its
	// schema; only rules that span tools belong here.
	Guidelines []string
	// AccessFor describes the effects of one validated call. A nil function is
	// treated as unknown access and therefore requires approval.
	AccessFor func(args map[string]any) []permission.Access
}

// Accesses returns the declared effects of one validated call.
func (t Tool) Accesses(args map[string]any) []permission.Access {
	if t.AccessFor == nil {
		return nil
	}
	return t.AccessFor(args)
}

// Name returns the tool's advertised name.
func (t Tool) Name() string { return t.Definition.Name }

// CodingTools returns the default v1 tool set rooted at the given workspace
// directory and backed by ops. One file-state store is shared by Read, Edit,
// and Write for this tool-set lifetime. Pass LocalOps{} for the local filesystem
// and shell. Background shells started by this set are abandoned when it is
// discarded; use CodingToolsWithShells when the caller needs to stop them.
func CodingTools(root string, ops Ops) []Tool {
	set, _ := CodingToolsWithShells(root, ops)
	return set
}

// CodingToolsWithShells is CodingTools plus the BackgroundShells manager backing
// the bash run_in_background workflow, so the caller can Shutdown any long-lived
// processes when the session ends.
func CodingToolsWithShells(
	root string,
	ops Ops,
	browserControllers ...BrowserController,
) ([]Tool, *BackgroundShells) {
	files := NewFileStateStore()
	shells := NewBackgroundShells()
	var inspectors []BrowserInspector
	if len(browserControllers) > 0 {
		if inspector, ok := browserControllers[0].(BrowserInspector); ok {
			inspectors = append(inspectors, inspector)
		}
	}
	return []Tool{
		Read(root, ops, files),
		Grep(root, ops),
		Glob(root, ops),
		LS(root, ops),
		Edit(root, ops, files),
		Write(root, ops, files),
		Bash(root, ops, shells),
		BashOutput(shells),
		OpenPreview(root, browserControllers...),
		InspectBrowser(inspectors...),
		KillBash(shells),
	}, shells
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

func pathAccess(action permission.Action) func(map[string]any) []permission.Access {
	return func(args map[string]any) []permission.Access {
		path, _ := args["path"].(string)
		return []permission.Access{{Action: action, Path: path}}
	}
}

func commandAccess(args map[string]any) []permission.Access {
	command, _ := args["command"].(string)
	return []permission.Access{{Action: permission.Execute, Command: command}}
}

// InternalAccess describes a tool that only interacts with state already owned
// by the coding session, such as buffered shell output or loaded skill text.
func InternalAccess(map[string]any) []permission.Access {
	return []permission.Access{{Action: permission.Internal}}
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
