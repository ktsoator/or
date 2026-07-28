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

// CoreToolsWithTasks returns the filesystem and shell tools that make up the
// coding product's core capability. Browser tools are assembled separately so
// the engine can register them as an optional capability without coupling that
// distinction to the reusable agent package.
func CoreToolsWithTasks(root string, ops Ops) ([]Tool, *TaskManager) {
	files := NewFileStateStore()
	tasks := NewTaskManager()
	return []Tool{
		Read(root, ops, files, tasks.OwnsOutputPath),
		Grep(root, ops),
		Glob(root, ops),
		Edit(root, ops, files),
		Write(root, ops, files),
		Bash(root, ops, tasks),
		TaskStop(tasks),
	}, tasks
}

// BrowserTools returns the browser bridge tools. The tools are present even
// when the controller is nil; in that configuration they fail closed,
// preserving the default session contract while making the capability boundary
// explicit.
func BrowserTools(root string, browserControllers ...BrowserController) []Tool {
	var inspectors []BrowserInspector
	var tabProviders []BrowserTabsProvider
	if len(browserControllers) > 0 {
		if inspector, ok := browserControllers[0].(BrowserInspector); ok {
			inspectors = append(inspectors, inspector)
		}
		if provider, ok := browserControllers[0].(BrowserTabsProvider); ok {
			tabProviders = append(tabProviders, provider)
		}
	}
	return []Tool{
		OpenPreview(root, browserControllers...),
		BrowserTabs(tabProviders...),
		InspectBrowser(inspectors...),
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
// by the coding session, such as managed task state or loaded skill text.
func InternalAccess(map[string]any) []permission.Access {
	return []permission.Access{{Action: permission.Internal}}
}

// textResult builds a ToolResult carrying a single text block.
func textResult(text string) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess},
	}
}

// resultWith builds a successful ToolResult whose model-facing text is derived
// from structured outcome data. The text is what the model reads; Data is the
// source of truth product shells render.
func resultWith(text string, data any) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: data},
	}
}

func failedResult(code, text string, data any) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{
			Status:    agent.ToolOutcomeFailed,
			ErrorCode: code,
			Data:      data,
		},
	}
}

func cancelledResult(code, text string, data any) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{
			Status:    agent.ToolOutcomeCancelled,
			ErrorCode: code,
			Data:      data,
		},
	}
}

func timeoutResult(code, text string, data any) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{
			Status:    agent.ToolOutcomeTimeout,
			ErrorCode: code,
			Data:      data,
		},
	}
}

func commandResult(status agent.ToolOutcomeStatus, code, text string, exitCode int) agent.ToolResult {
	return agent.ToolResult{
		Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
		Outcome: agent.ToolOutcome{
			Status:    status,
			ErrorCode: code,
			ExitCode:  &exitCode,
		},
	}
}

// mutationFailure builds a failed edit/write result carrying both the text
// detail the model reads and a structured MutationFailure product shells render.
func mutationFailure(path, reason, detail string) agent.ToolResult {
	return failedResult(reason, detail, MutationFailure{Path: path, Reason: reason, Detail: detail})
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
