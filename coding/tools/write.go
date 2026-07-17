package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

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

// WriteResult is the UI-independent outcome of a full-file write.
type WriteResult struct {
	Path    string
	Created bool
	Bytes   int
}

// Write returns a tool that writes a file in full, creating parent directories
// as needed and overwriting any existing file. It runs sequentially with other
// tool calls so concurrent writes cannot corrupt a file. Use Edit for targeted
// changes to an existing file.
func Write(root string, ops FileOps, files *FileStateStore) Tool {
	def := llm.MustTool[writeArgs]("write", writeText.description)
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
				if strings.TrimSpace(in.Path) == "" {
					err := fmt.Errorf("write: path is required")
					return textResult(err.Error()), err
				}
				if err := ctx.Err(); err != nil {
					return textResult("write: " + err.Error()), err
				}
				path := resolve(root, in.Path)
				existed, err := checkWriteTarget(ctx, ops, files, path)
				if err != nil {
					err = mutationStateError("write", in.Path, err)
					return textResult(err.Error()), err
				}
				if err := ops.MkdirAll(ctx, filepath.Dir(path), defaultDirPerm); err != nil {
					return textResult(fmt.Sprintf("write %s: %v", in.Path, err)), err
				}
				// Check again immediately before the mutation. In particular, a path
				// that did not exist above may have been created concurrently.
				currentExists, err := checkWriteTarget(ctx, ops, files, path)
				if err != nil {
					err = mutationStateError("write", in.Path, err)
					return textResult(err.Error()), err
				}
				if currentExists != existed {
					err := mutationStateError("write", in.Path, ErrFileChanged)
					return textResult(err.Error()), err
				}
				if err := ctx.Err(); err != nil {
					return textResult("write: " + err.Error()), err
				}
				if err := ops.WriteFile(ctx, path, []byte(in.Content), defaultFilePerm); err != nil {
					return textResult(fmt.Sprintf("write %s: %v", in.Path, err)), err
				}
				if info, err := ops.Stat(ctx, path); err == nil {
					files.Record(path, info)
				} else {
					files.Delete(path)
				}
				result := WriteResult{Path: in.Path, Created: !existed, Bytes: len(in.Content)}
				return textResult(formatWriteResult(result)), nil
			},
		},
		PromptSnippet: writeText.snippet,
		Guidelines:    writeText.guidelines,
	}
}

// checkWriteTarget allows a new path, but requires an existing path to match a
// previously observed version.
func checkWriteTarget(ctx context.Context, ops FileOps, files *FileStateStore, path string) (bool, error) {
	info, err := ops.Stat(ctx, path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := files.Check(path, info); err != nil {
		return true, err
	}
	return true, nil
}

func formatWriteResult(result WriteResult) string {
	action := "Updated"
	if result.Created {
		action = "Created"
	}
	return fmt.Sprintf("%s %s (%d bytes).", action, result.Path, result.Bytes)
}
