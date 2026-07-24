package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/llm"
)

// defaultGrepLimit caps grep output when the model does not set a limit.
const defaultGrepLimit = 250

type grepArgs struct {
	Pattern    string `json:"pattern" jsonschema:"description=Regular expression (Go regexp syntax) to search for,minLength=1"`
	Path       string `json:"path,omitempty" jsonschema:"description=Subdirectory to search; defaults to the workspace root"`
	Glob       string `json:"glob,omitempty" jsonschema:"description=Filter files by a glob pattern such as *.go or **/*.ts"`
	Mode       string `json:"mode,omitempty" jsonschema:"description=Output mode: files (paths of matching files, the default) or content (matching lines with line numbers),enum=files,enum=content"`
	IgnoreCase bool   `json:"ignore_case,omitempty" jsonschema:"description=Case-insensitive match"`
	Limit      int    `json:"limit,omitempty" jsonschema:"description=Maximum files (files mode) or lines (content mode) to return; defaults to 250,minimum=1"`
}

// Grep returns a tool that searches file contents across the workspace with a
// regular expression, skipping vendored directories. It returns matching file
// paths by default, or matching lines in content mode.
func Grep(root string, ops FileOps) Tool {
	def := llm.MustTool[grepArgs]("grep", grepText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition: def,
			Label:      "Grep",
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in grepArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				return runGrep(ctx, root, ops, in)
			},
		},
		AccessFor:  pathAccess(permission.Read),
		Guidelines: grepText.guidelines,
	}
}

func runGrep(ctx context.Context, root string, ops FileOps, in grepArgs) (agent.ToolResult, error) {
	expr := in.Pattern
	if in.IgnoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		msg := fmt.Sprintf("grep: invalid pattern: %v", err)
		return textResult(msg), fmt.Errorf("%s", msg)
	}

	var globRe *regexp.Regexp
	if in.Glob != "" {
		globRe, err = globToRegexp(in.Glob)
		if err != nil {
			msg := fmt.Sprintf("grep: invalid glob: %v", err)
			return textResult(msg), fmt.Errorf("%s", msg)
		}
	}

	searchRoot := resolve(root, in.Path)
	files, err := walkFiles(ctx, ops, searchRoot)
	if err != nil {
		return textResult(fmt.Sprintf("grep: %v", err)), err
	}

	limit := in.Limit
	if limit <= 0 {
		limit = defaultGrepLimit
	}
	contentMode := in.Mode == "content"

	var lines []string
	truncated := false
	for _, f := range files {
		if globRe != nil && !globRe.MatchString(f.rel) {
			continue
		}
		data, err := ops.ReadFile(ctx, filepath.Join(searchRoot, filepath.FromSlash(f.rel)))
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			continue // unreadable or binary; skip
		}
		disp := displayPath(root, searchRoot, f.rel)

		if !contentMode {
			if re.Match(data) {
				lines = append(lines, disp)
			}
		} else {
			for n, line := range strings.Split(string(data), "\n") {
				if re.MatchString(line) {
					lines = append(lines, fmt.Sprintf("%s:%d:%s", disp, n+1, line))
				}
			}
		}

		if len(lines) >= limit {
			lines = lines[:limit]
			truncated = true
			break
		}
	}

	if len(lines) == 0 {
		return textResult("No matches found."), nil
	}
	out := strings.Join(lines, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n[truncated at %d results; refine the pattern or set a higher limit]", limit)
	}
	return textResult(out), nil
}

// displayPath renders a file found under searchRoot (relative path f) as a path
// relative to the workspace root, so it can be passed straight to read or edit.
func displayPath(root, searchRoot, rel string) string {
	full := filepath.Join(searchRoot, filepath.FromSlash(rel))
	disp, err := filepath.Rel(root, full)
	if err != nil {
		return rel
	}
	return filepath.ToSlash(disp)
}
