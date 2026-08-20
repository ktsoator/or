package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ktsoator/or/llm"
)

func TestRunLoopGetAPIKeyOverridesPerStep(t *testing.T) {
	var seenKey string
	streamFn := func(_ context.Context, _ llm.Model, _ llm.Context, options llm.StreamOptions) (<-chan llm.Event, error) {
		seenKey = options.APIKey
		ch := make(chan llm.Event, 1)
		ch <- done(textAssistant("ok"))
		close(ch)
		return ch, nil
	}
	cfg := LoopConfig{
		Model:     testModel,
		StreamFn:  streamFn,
		GetAPIKey: func(provider string) string { return "key-for-" + provider },
	}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("hi")}, Context{}, cfg))

	if seenKey != "key-for-p" {
		t.Fatalf("api key passed to stream = %q, want %q", seenKey, "key-for-p")
	}
}

func TestRunLoopPrepareArgumentsRewritesBeforeValidation(t *testing.T) {
	executed := false
	seen := ""
	tool := AgentTool{
		Definition: llm.MustTool[echoArgs]("echo", "echo"),
		// The model sends empty arguments, which would fail validation. Fill in
		// the required field before validation.
		PrepareArguments: func(map[string]any) map[string]any {
			return map[string]any{"text": "prepared"}
		},
		Execute: func(_ context.Context, _ string, args json.RawMessage, _ func(ToolProgress)) (ToolResult, error) {
			executed = true
			var parsed echoArgs
			_ = json.Unmarshal(args, &parsed)
			seen = parsed.Text
			return ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "ok"}}}, nil
		},
	}
	rec := &recorder{steps: [][]llm.Event{
		{done(toolCallAssistant("c1", "echo", map[string]any{}))},
		{done(textAssistant("done"))},
	}}
	cfg := LoopConfig{Model: testModel, StreamFn: rec.fn()}
	base := Context{Tools: []AgentTool{tool}}

	collect(RunLoop(context.Background(), []AgentMessage{userPrompt("go")}, base, cfg))

	if !executed {
		t.Fatal("tool should have executed after PrepareArguments filled the field")
	}
	if seen != "prepared" {
		t.Fatalf("tool saw arguments %q, want %q", seen, "prepared")
	}
}
