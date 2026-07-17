package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

// defaultGlobLimit caps how many matching paths glob returns.
const defaultGlobLimit = 100

type globArgs struct {
	Pattern string `json:"pattern" jsonschema:"description=Glob pattern to match file paths against, e.g. **/*.go or cmd/**/main.go,minLength=1"`
	Path    string `json:"path,omitempty" jsonschema:"description=Subdirectory to search under; defaults to the workspace root"`
}

// Glob returns a tool that finds files by name pattern, skipping vendored
// directories, and returns paths sorted by most-recently-modified first.
func Glob(root string, ops FileOps) Tool {
	def := llm.MustTool[globArgs]("glob", globText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Glob",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in globArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				return runGlob(ctx, root, ops, in)
			},
		},
		ReadOnly:      true,
		PromptSnippet: globText.snippet,
		Guidelines:    globText.guidelines,
	}
}

func runGlob(ctx context.Context, root string, ops FileOps, in globArgs) (agent.ToolResult, error) {
	re, err := globToRegexp(in.Pattern)
	if err != nil {
		msg := fmt.Sprintf("glob: invalid pattern: %v", err)
		return textResult(msg), fmt.Errorf("%s", msg)
	}

	searchRoot := resolve(root, in.Path)
	files, err := walkFiles(ctx, ops, searchRoot)
	if err != nil {
		return textResult(fmt.Sprintf("glob: %v", err)), err
	}

	matches := files[:0:0]
	for _, f := range files {
		if re.MatchString(f.rel) {
			matches = append(matches, f)
		}
	}

	// Most-recently-modified first, so the files likely being worked on lead.
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].info.ModTime().After(matches[j].info.ModTime())
	})

	truncated := false
	if len(matches) > defaultGlobLimit {
		matches = matches[:defaultGlobLimit]
		truncated = true
	}

	if len(matches) == 0 {
		return textResult("No files found."), nil
	}
	paths := make([]string, len(matches))
	for i, f := range matches {
		paths[i] = displayPath(root, searchRoot, f.rel)
	}
	out := strings.Join(paths, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n[truncated at %d files; use a more specific pattern or path]", defaultGlobLimit)
	}
	return textResult(out), nil
}
