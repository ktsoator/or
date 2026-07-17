package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type readArgs struct {
	Path   string `json:"path" jsonschema:"description=Path to the file to read, absolute or relative to the workspace root,minLength=1"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=1-based line number to start reading from,minimum=1"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to read,minimum=1"`
}

// Read returns a tool that reads a UTF-8 text file and returns its contents with
// 1-based line numbers, optionally windowed by offset and limit. Output is
// capped to keep a large file from filling the context window.
func Read(root string, ops FileOps) Tool {
	def := llm.MustTool[readArgs]("read", readText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Read",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in readArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				path := resolve(root, in.Path)
				data, err := ops.ReadFile(ctx, path)
				if err != nil {
					return textResult(fmt.Sprintf("read %s: %v", in.Path, err)), err
				}

				lines := strings.Split(string(data), "\n")
				start := 0
				if in.Offset > 0 {
					start = in.Offset - 1
				}
				if start > len(lines) {
					start = len(lines)
				}
				end := len(lines)
				if in.Limit > 0 && start+in.Limit < end {
					end = start + in.Limit
				}

				var b strings.Builder
				for i := start; i < end; i++ {
					fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
				}
				return textResult(truncate(b.String(), DefaultMaxLines, DefaultMaxBytes)), nil
			},
		},
		ReadOnly:      true,
		PromptSnippet: readText.snippet,
		Guidelines:    readText.guidelines,
	}
}
