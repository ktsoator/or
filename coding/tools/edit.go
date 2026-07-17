package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type editArgs struct {
	Path       string `json:"path" jsonschema:"description=Path to the file to edit,minLength=1"`
	OldString  string `json:"old_string" jsonschema:"description=Exact text to replace; must match the file uniquely unless replace_all is set,minLength=1"`
	NewString  string `json:"new_string" jsonschema:"description=Replacement text"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace every occurrence instead of requiring a unique match"`
}

// Edit returns a tool that replaces an exact substring in a file. By default the
// match must be unique, so an ambiguous edit fails instead of changing the wrong
// place; set replace_all to change every occurrence. It runs sequentially with
// other tool calls so concurrent edits cannot corrupt a file.
func Edit(root string, ops FileOps, files *FileStateStore) Tool {
	def := llm.MustTool[editArgs]("edit", editText.description)
	return Tool{
		AgentTool: agent.AgentTool{
			Definition:    def,
			Label:         "Edit",
			ExecutionMode: agent.ExecutionSequential,
			Execute: func(ctx context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in editArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				if in.OldString == in.NewString {
					err := fmt.Errorf("old_string and new_string are identical")
					return mutationFailure(in.Path, FailureInput, "edit: "+err.Error()), err
				}

				path := resolve(root, in.Path)
				info, err := ops.Stat(ctx, path)
				if err != nil {
					detail := fmt.Sprintf("edit %s: %v", in.Path, err)
					return mutationFailure(in.Path, statFailureReason(err), detail), err
				}
				if err := files.Check(path, info); err != nil {
					reason := stateFailureReason(err)
					err = mutationStateError("edit", in.Path, err)
					return mutationFailure(in.Path, reason, err.Error()), err
				}
				data, err := ops.ReadFile(ctx, path)
				if err != nil {
					detail := fmt.Sprintf("edit %s: %v", in.Path, err)
					return mutationFailure(in.Path, FailureIO, detail), err
				}
				content := string(data)

				count := strings.Count(content, in.OldString)
				switch {
				case count == 0:
					err := fmt.Errorf("old_string not found in %s", in.Path)
					return mutationFailure(in.Path, FailureNotFound, "edit: "+err.Error()), err
				case count > 1 && !in.ReplaceAll:
					err := fmt.Errorf("old_string matches %d places in %s; make it unique or set replace_all", count, in.Path)
					return mutationFailure(in.Path, FailureAmbiguous, "edit: "+err.Error()), err
				}

				updated := strings.ReplaceAll(content, in.OldString, in.NewString)
				current, err := ops.Stat(ctx, path)
				if err != nil {
					detail := fmt.Sprintf("edit %s: %v", in.Path, err)
					return mutationFailure(in.Path, statFailureReason(err), detail), err
				}
				if err := files.Check(path, current); err != nil {
					reason := stateFailureReason(err)
					err = mutationStateError("edit", in.Path, err)
					return mutationFailure(in.Path, reason, err.Error()), err
				}
				var perm os.FileMode = current.Mode().Perm()
				if err := ops.WriteFile(ctx, path, []byte(updated), perm); err != nil {
					detail := fmt.Sprintf("edit %s: %v", in.Path, err)
					return mutationFailure(in.Path, FailureIO, detail), err
				}
				if updatedInfo, err := ops.Stat(ctx, path); err == nil {
					files.Record(path, updatedInfo)
				} else {
					files.Delete(path)
				}

				change := FileChange{Path: in.Path, Kind: ChangeUpdate, Bytes: len(updated)}
				change.Hunks, change.Additions, change.Deletions = diffLines(content, updated)
				return resultWith(formatEditResult(in.Path, count, change), change), nil
			},
		},
		PromptSnippet: editText.snippet,
		Guidelines:    editText.guidelines,
	}
}

// formatEditResult is the model-facing summary derived from the structured
// change.
func formatEditResult(path string, count int, c FileChange) string {
	return fmt.Sprintf("Edited %s (%d replacement(s), +%d -%d).", path, count, c.Additions, c.Deletions)
}
