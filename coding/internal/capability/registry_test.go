package capability

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func testTool(name string) tools.Tool {
	return tools.Tool{AgentTool: agent.AgentTool{
		Definition: llm.ToolDefinition{Name: name},
		Execute: func(context.Context, string, json.RawMessage, func(agent.ToolProgress)) (agent.ToolResult, error) {
			return agent.ToolResult{}, nil
		},
	}}
}

func TestRegistryRegistersAndReplacesToolsDeterministically(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Manifest: Manifest{ID: "core", Version: "1"},
		Tools: []ToolContribution{
			{Tool: testTool("read")},
			{Tool: testTool("write")},
		},
		PromptSections: []string{"## Core", "  ## Shared  "},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Definition{
		Manifest: Manifest{ID: "project", Version: "2"},
		Tools: []ToolContribution{
			{Tool: testTool("write"), Replace: true},
			{Tool: testTool("deploy")},
		},
		PromptSections: []string{"## Shared", "## Project"},
	}); err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, tool := range registry.Tools() {
		names = append(names, tool.Name())
	}
	if !reflect.DeepEqual(names, []string{"read", "write", "deploy"}) {
		t.Fatalf("tool order = %v", names)
	}
	if source, _ := registry.ToolSource("write"); source != "project" {
		t.Fatalf("write source = %q, want project", source)
	}
	if got := registry.PromptSections(); !reflect.DeepEqual(got, []string{"## Core", "## Shared", "## Project"}) {
		t.Fatalf("prompt sections = %#v", got)
	}
}

func TestRegistryRejectsAmbiguousRegistrationsAtomically(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Definition{
		Manifest: Manifest{ID: "core"},
		Tools:    []ToolContribution{{Tool: testTool("read")}},
	}); err != nil {
		t.Fatal(err)
	}

	err := registry.Register(Definition{
		Manifest: Manifest{ID: "broken"},
		Tools: []ToolContribution{
			{Tool: testTool("write")},
			{Tool: testTool("read")},
		},
	})
	if err == nil {
		t.Fatal("duplicate tool registration succeeded")
	}
	if len(registry.Manifests()) != 1 || len(registry.Tools()) != 1 {
		t.Fatalf("failed registration mutated registry: manifests=%v tools=%v", registry.Manifests(), registry.Tools())
	}
}

func TestRegistryComposesToolHooksInRegistrationOrder(t *testing.T) {
	registry := NewRegistry()
	var beforeOrder []string
	args := map[string]any{"target": map[string]any{"environment": "production"}}
	if err := registry.Register(Definition{
		Manifest: Manifest{ID: "first"},
		BeforeToolCall: func(ctx agent.BeforeToolCallCtx) (bool, string) {
			beforeOrder = append(beforeOrder, "first")
			ctx.Args.(map[string]any)["target"].(map[string]any)["environment"] = "mutated"
			return false, ""
		},
		AfterToolCall: func(agent.AfterToolCallCtx) *agent.AfterToolCallResult {
			outcome := agent.ToolOutcome{Status: agent.ToolOutcomeSuccess, Data: "first"}
			return &agent.AfterToolCallResult{Outcome: &outcome}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(Definition{
		Manifest: Manifest{ID: "second"},
		BeforeToolCall: func(ctx agent.BeforeToolCallCtx) (bool, string) {
			beforeOrder = append(beforeOrder, "second")
			if got := ctx.Args.(map[string]any)["target"].(map[string]any)["environment"]; got != "production" {
				t.Fatalf("second hook observed mutated arguments: %v", got)
			}
			return true, "blocked"
		},
		AfterToolCall: func(ctx agent.AfterToolCallCtx) *agent.AfterToolCallResult {
			if ctx.Result.Outcome.Data != "first" {
				t.Fatalf("second hook outcome = %#v", ctx.Result.Outcome)
			}
			outcome := agent.ToolOutcome{
				Status:    agent.ToolOutcomeFailed,
				ErrorCode: "hook_rejected",
				Data:      "second",
			}
			return &agent.AfterToolCallResult{Outcome: &outcome}
		},
	}); err != nil {
		t.Fatal(err)
	}

	if block, reason := registry.BeforeToolCall()(agent.BeforeToolCallCtx{Args: args}); !block || reason != "blocked" {
		t.Fatalf("before result = (%v, %q)", block, reason)
	}
	if !reflect.DeepEqual(beforeOrder, []string{"first", "second"}) {
		t.Fatalf("before order = %v", beforeOrder)
	}
	if got := args["target"].(map[string]any)["environment"]; got != "production" {
		t.Fatalf("before hook mutated original arguments: %v", got)
	}

	override := registry.AfterToolCall()(agent.AfterToolCallCtx{})
	if override == nil || override.Outcome == nil ||
		override.Outcome.Status != agent.ToolOutcomeFailed ||
		override.Outcome.ErrorCode != "hook_rejected" ||
		override.Outcome.Data != "second" {
		t.Fatalf("after override = %#v", override)
	}
}
