package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

type lsArgs struct {
	Path string `json:"path,omitempty" jsonschema:"description=Directory to list; defaults to the workspace root"`
}

// LS returns a tool that lists a single directory's entries, directories first,
// each directory suffixed with a slash.
func LS(root string, ops FileOps) Tool {
	def := llm.MustTool[lsArgs]("ls", lsText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "LS",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in lsArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				dir := resolve(root, in.Path)
				entries, err := ops.ReadDir(ctx, dir)
				if err != nil {
					return textResult(fmt.Sprintf("ls %s: %v", in.Path, err)), err
				}

				names := make([]string, 0, len(entries))
				for _, e := range entries {
					if e.IsDir() {
						names = append(names, e.Name()+"/")
					} else {
						names = append(names, e.Name())
					}
				}
				// Directories first (their names carry a trailing slash), then files,
				// each group alphabetical.
				sort.Slice(names, func(i, j int) bool {
					di := strings.HasSuffix(names[i], "/")
					dj := strings.HasSuffix(names[j], "/")
					if di != dj {
						return di
					}
					return names[i] < names[j]
				})

				if len(names) == 0 {
					return textResult("(empty directory)"), nil
				}
				return textResult(strings.Join(names, "\n")), nil
			},
		},
		AccessFor:     pathAccess(permission.Read),
		PromptSnippet: lsText.snippet,
		Guidelines:    lsText.guidelines,
	}
}
