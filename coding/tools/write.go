package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// File and directory permissions for newly created workspace entries.
const (
	defaultFilePerm = 0o644
	defaultDirPerm  = 0o755
)

type writeArgs struct {
	Path    string `json:"path" jsonschema:"description=Path to the file to write; parent directories are created as needed,minLength=1"`
	Content string `json:"content" jsonschema:"description=Full contents to write to the file"`
}

// Write returns a tool that writes a file in full, creating parent directories
// as needed and overwriting any existing file. It runs sequentially with other
// tool calls so concurrent writes cannot corrupt a file. Use Edit for targeted
// changes to an existing file.
func Write(root string, ops FileOps) Tool {
	def := llm.MustTool[writeArgs]("write", "Write a file in full, creating or overwriting it. Use edit for targeted changes.")
	return Tool{
		AgentTool: agent.AgentTool{
			Definition:    def,
			Label:         "Write",
			ExecutionMode: agent.ExecutionSequential,
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in writeArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				path := resolve(root, in.Path)
				if err := ops.MkdirAll(ctx, filepath.Dir(path), defaultDirPerm); err != nil {
					return textResult(fmt.Sprintf("write %s: %v", in.Path, err)), err
				}
				if err := ops.WriteFile(ctx, path, []byte(in.Content), defaultFilePerm); err != nil {
					return textResult(fmt.Sprintf("write %s: %v", in.Path, err)), err
				}
				return textResult(fmt.Sprintf("Wrote %s (%d bytes).", in.Path, len(in.Content))), nil
			},
		},
		PromptSnippet: "write: create or overwrite a file in full.",
		Guidelines: []string{
			"Prefer edit over write when changing part of an existing file.",
		},
	}
}
