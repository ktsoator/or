package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/internal/llm"
)

type recordingTool struct {
	called    bool
	arguments map[string]any
}

func (tool *recordingTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name: "record",
		Parameters: json.RawMessage(`{
			"type":"object",
			"required":["count"],
			"additionalProperties":false,
			"properties":{"count":{"type":"integer","minimum":1}}
		}`),
	}
}

func (tool *recordingTool) Execute(_ context.Context, arguments map[string]any) (Result, error) {
	tool.called = true
	tool.arguments = arguments
	return Result{Content: []llm.ToolResultContent{&llm.TextContent{Text: "ok"}}}, nil
}

func TestRunToolValidatesAndUsesCoercedArguments(t *testing.T) {
	tool := &recordingTool{}
	loop := loop{cfg: loopConfig{tools: map[string]Tool{"record": tool}}}

	_, isError := loop.runTool(context.Background(), &llm.ToolCall{
		Name:      "record",
		Arguments: map[string]any{"count": "2"},
	})
	if isError {
		t.Fatal("runTool() returned an error for coercible arguments")
	}
	if !tool.called {
		t.Fatal("runTool() did not execute the tool")
	}
	if got := tool.arguments["count"]; got != float64(2) {
		t.Fatalf("tool received count = %#v, want float64(2)", got)
	}
}

func TestRunToolRejectsInvalidArgumentsBeforeExecute(t *testing.T) {
	tool := &recordingTool{}
	loop := loop{cfg: loopConfig{tools: map[string]Tool{"record": tool}}}

	result, isError := loop.runTool(context.Background(), &llm.ToolCall{
		Name:      "record",
		Arguments: map[string]any{"count": float64(0)},
	})
	if !isError {
		t.Fatal("runTool() accepted invalid arguments")
	}
	if tool.called {
		t.Fatal("runTool() executed the tool with invalid arguments")
	}
	if len(result.Content) != 1 {
		t.Fatalf("error result content length = %d, want 1", len(result.Content))
	}
}
