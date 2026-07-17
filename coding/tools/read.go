package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

const (
	maxReadLines     = 2000
	readNoticeBudget = 256
)

type readArgs struct {
	Path   string `json:"path" jsonschema:"description=Path to the file to read, absolute or relative to the workspace root,minLength=1"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=1-based line number to start reading from,minimum=1"`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to read,minimum=1"`
}

// ReadResult is the provider- and UI-independent result of a text range read.
// Content does not include line-number prefixes; those are added only when the
// result is serialized for the model.
type ReadResult struct {
	Path       string
	Content    string
	StartLine  int
	LineCount  int
	Limit      int
	HasMore    bool
	NextOffset int
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
				offset, limit, err := normalizeReadArgs(in)
				if err != nil {
					return textResult(err.Error()), err
				}
				path := resolve(root, in.Path)
				file, err := ops.Open(ctx, path)
				if err != nil {
					return textResult(fmt.Sprintf("read %s: %v", in.Path, err)), err
				}
				defer file.Close()

				result, err := readTextRange(ctx, file, offset, limit, DefaultMaxBytes-readNoticeBudget)
				if err != nil {
					msg := fmt.Sprintf("read %s: %v", in.Path, err)
					return textResult(msg), err
				}
				result.Path = in.Path
				return textResult(formatReadResult(result)), nil
			},
		},
		ReadOnly:      true,
		PromptSnippet: readText.snippet,
		Guidelines:    readText.guidelines,
	}
}

func normalizeReadArgs(in readArgs) (offset, limit int, err error) {
	if strings.TrimSpace(in.Path) == "" {
		return 0, 0, fmt.Errorf("read: path is required")
	}
	offset = in.Offset
	if offset == 0 {
		offset = 1
	}
	if offset < 1 {
		return 0, 0, fmt.Errorf("read: offset must be at least 1")
	}
	limit = in.Limit
	if limit == 0 {
		limit = DefaultMaxLines
	}
	if limit < 1 {
		return 0, 0, fmt.Errorf("read: limit must be at least 1")
	}
	if limit > maxReadLines {
		return 0, 0, fmt.Errorf("read: limit must not exceed %d", maxReadLines)
	}
	return offset, limit, nil
}

// readTextRange reads at most limit complete lines beginning at the 1-based
// offset. It keeps memory bounded by maxBytes and reads one extra line only to
// determine whether the model can continue with NextOffset.
func readTextRange(ctx context.Context, src io.Reader, offset, limit, maxBytes int) (ReadResult, error) {
	result := ReadResult{StartLine: offset, Limit: limit}
	reader := bufio.NewReader(src)

	for lineNumber := 1; lineNumber < offset; lineNumber++ {
		if err := ctx.Err(); err != nil {
			return ReadResult{}, err
		}
		_, ok, _, err := readCompleteLine(reader, -1)
		if err != nil {
			return ReadResult{}, err
		}
		if !ok {
			return result, nil
		}
	}

	lines := make([]string, 0, limit)
	bodyBytes := 0
	for len(lines) < limit {
		if err := ctx.Err(); err != nil {
			return ReadResult{}, err
		}
		lineNumber := offset + len(lines)
		line, ok, overflow, err := readCompleteLine(reader, maxBytes)
		if err != nil {
			return ReadResult{}, err
		}
		if !ok {
			break
		}
		if overflow {
			if len(lines) == 0 {
				return ReadResult{}, fmt.Errorf("line %d exceeds the %d-byte read output limit; use grep or bash to inspect it", lineNumber, DefaultMaxBytes)
			}
			result.HasMore = true
			break
		}

		formattedBytes := len(fmt.Sprintf("%6d\t", lineNumber)) + len(line) + 1
		if bodyBytes+formattedBytes > maxBytes {
			if len(lines) == 0 {
				return ReadResult{}, fmt.Errorf("line %d exceeds the %d-byte read output limit; use grep or bash to inspect it", lineNumber, DefaultMaxBytes)
			}
			result.HasMore = true
			break
		}
		lines = append(lines, line)
		bodyBytes += formattedBytes
	}

	result.Content = strings.Join(lines, "\n")
	result.LineCount = len(lines)
	result.NextOffset = offset + result.LineCount
	if result.HasMore {
		return result, nil
	}

	if len(lines) == limit {
		if err := ctx.Err(); err != nil {
			return ReadResult{}, err
		}
		_, ok, _, err := readCompleteLine(reader, -1)
		if err != nil {
			return ReadResult{}, err
		}
		result.HasMore = ok
	}
	return result, nil
}

// readCompleteLine reads one logical line without bufio.Scanner's token limit.
// captureLimit < 0 discards the content while still advancing to the next line.
func readCompleteLine(reader *bufio.Reader, captureLimit int) (line string, ok, overflow bool, err error) {
	var b strings.Builder
	seen := false
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			seen = true
			hasNewline := fragment[len(fragment)-1] == '\n'
			if hasNewline {
				fragment = fragment[:len(fragment)-1]
			}
			if captureLimit >= 0 && !overflow {
				if b.Len()+len(fragment) > captureLimit {
					overflow = true
				} else {
					b.Write(fragment)
				}
			}
			if hasNewline {
				break
			}
		}

		switch readErr {
		case nil:
			break
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if !seen {
				return "", false, false, nil
			}
			break
		default:
			return "", false, false, readErr
		}
		break
	}

	if captureLimit >= 0 && !overflow {
		line = strings.TrimSuffix(b.String(), "\r")
	}
	return line, true, overflow, nil
}

func formatReadResult(result ReadResult) string {
	if result.LineCount == 0 {
		if result.StartLine == 1 {
			return "(empty file)"
		}
		return fmt.Sprintf("[No content at or after line %d.]", result.StartLine)
	}

	lines := strings.Split(result.Content, "\n")
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%6d\t%s\n", result.StartLine+i, line)
	}
	if result.StartLine > 1 || result.HasMore {
		fmt.Fprintf(
			&b,
			"\n[Showing lines %d-%d. This is a partial view; use offset and limit to read any other range.]",
			result.StartLine,
			result.StartLine+result.LineCount-1,
		)
	}
	return b.String()
}
