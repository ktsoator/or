package harness_test

import (
	"context"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/agent/harness"
	"github.com/ktsoator/or/llm"
)

func TestInitialActiveToolsSubset(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{
		Model:       testModel,
		Tools:       []agent.AgentTool{namedTool("alpha"), namedTool("beta")},
		ActiveTools: []string{"beta"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	active := h.ActiveTools()
	if len(active) != 1 || active[0].Definition.Name != "beta" {
		t.Fatalf("active tools = %v, want [beta]", toolNames(active))
	}
	if got := len(h.Tools()); got != 2 {
		t.Fatalf("registry has %d tools, want 2", got)
	}
}

func TestSetActiveToolsRestrictsAdvertised(t *testing.T) {
	ctx := context.Background()
	rec := &recordingStream{turns: [][]llm.Event{textTurn("ok")}}
	h, err := harness.New(ctx, harness.Options{
		Model:    testModel,
		StreamFn: rec.fn(),
		Tools:    []agent.AgentTool{namedTool("alpha"), namedTool("beta")},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := len(h.ActiveTools()); got != 2 {
		t.Fatalf("initial active tools = %d, want 2 (all)", got)
	}

	if err := h.SetActiveTools("alpha"); err != nil {
		t.Fatalf("SetActiveTools() error = %v", err)
	}
	if err := h.Prompt(ctx, "hi"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(rec.toolNames) != 1 {
		t.Fatalf("model ran %d times, want 1", len(rec.toolNames))
	}
	if got := rec.toolNames[0]; len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("model advertised tools = %v, want [alpha]", got)
	}
}

func TestSetActiveToolsUnknownErrors(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{
		Model: testModel,
		Tools: []agent.AgentTool{namedTool("alpha")},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := h.SetActiveTools("ghost"); err == nil {
		t.Fatal("SetActiveTools(\"ghost\") = nil, want error for unknown tool")
	}
	// The active set is left unchanged on error.
	if got := len(h.ActiveTools()); got != 1 {
		t.Fatalf("active tools = %d after failed SetActiveTools, want 1", got)
	}
}

func TestSetActiveToolsResetReactivatesAll(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{
		Model:       testModel,
		Tools:       []agent.AgentTool{namedTool("alpha"), namedTool("beta")},
		ActiveTools: []string{"alpha"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := h.SetActiveTools(); err != nil {
		t.Fatalf("SetActiveTools() error = %v", err)
	}
	if got := len(h.ActiveTools()); got != 2 {
		t.Fatalf("active tools = %d after reset, want 2 (all)", got)
	}
}

func TestSetModelUpdatesSnapshot(t *testing.T) {
	ctx := context.Background()
	h, err := harness.New(ctx, harness.Options{Model: testModel, StreamFn: scriptedStream("x")})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	other := llm.Model{ID: "other", Provider: "p", Protocol: llm.ProtocolOpenAICompletions}
	h.SetModel(other)
	if got := h.Snapshot().Model.ID; got != "other" {
		t.Fatalf("model ID = %q after SetModel, want \"other\"", got)
	}
}

func toolNames(tools []agent.AgentTool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Definition.Name
	}
	return names
}
