package engine

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/coding/internal/capability"
	"github.com/ktsoator/or/coding/internal/permission"
	"github.com/ktsoator/or/coding/internal/tools"
	"github.com/ktsoator/or/llm"
)

func TestSessionAssemblesCapabilityToolsAndPromptSections(t *testing.T) {
	session, err := New(context.Background(), Options{
		Model: llm.Model{Provider: "test", ID: "model"},
		Tools: []tools.Tool{},
		Capabilities: []capability.Definition{{
			Manifest: capability.Manifest{ID: "project.deploy", Version: "1"},
			Tools: []capability.ToolContribution{{
				Tool: capabilityTestTool("deploy", nil),
			}},
			PromptSections: []string{"## Deploy policy\nUse staging first."},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	state := session.agent.Snapshot()
	var names []string
	for _, tool := range state.Tools {
		names = append(names, tool.Definition.Name)
	}
	if !reflect.DeepEqual(names, []string{"skill", "deploy"}) {
		t.Fatalf("tools = %v, want stable skill plus contributed deploy", names)
	}
	if !strings.Contains(state.SystemPrompt, "## Deploy policy\nUse staging first.") {
		t.Fatalf("system prompt omitted capability section:\n%s", state.SystemPrompt)
	}
}

func TestCapabilityVetoRunsBeforeCorePermission(t *testing.T) {
	executed := 0
	approver := &countingApprover{}
	modelCalls := 0
	session, err := New(context.Background(), Options{
		Model:    llm.Model{Provider: "test", ID: "model"},
		Tools:    []tools.Tool{},
		Approver: approver,
		Capabilities: []capability.Definition{{
			Manifest: capability.Manifest{ID: "project.guard", Version: "1"},
			Tools: []capability.ToolContribution{{
				Tool: capabilityTestTool("deploy", func() { executed++ }),
			}},
			BeforeToolCall: func(ctx agent.BeforeToolCallCtx) (bool, string) {
				return ctx.ToolCall.Name == "deploy", "deploy is disabled"
			},
		}},
		StreamFn: func(
			_ context.Context,
			model llm.Model,
			_ llm.Context,
			_ llm.StreamOptions,
		) (<-chan llm.Event, error) {
			modelCalls++
			if modelCalls == 1 {
				message := llm.NewAssistantMessage(model)
				message.StopReason = llm.StopReasonToolUse
				message.Content = []llm.AssistantContent{
					&llm.ToolCall{ID: "call-1", Name: "deploy", Arguments: map[string]any{}},
				}
				return finalEvents(llm.EventDone, &message), nil
			}
			return assistantEvents(model, "deploy remained disabled"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := session.Prompt(context.Background(), "deploy now"); err != nil {
		t.Fatal(err)
	}
	if executed != 0 {
		t.Fatalf("blocked capability tool executed %d times", executed)
	}
	if approver.calls != 0 {
		t.Fatalf("permission approver called %d times before capability veto", approver.calls)
	}
	if modelCalls != 2 {
		t.Fatalf("model calls = %d, want tool request plus final response", modelCalls)
	}
}

func capabilityTestTool(name string, onExecute func()) tools.Tool {
	return tools.Tool{AgentTool: agent.AgentTool{
		Definition: llm.MustTool[struct{}](name, "Test capability tool"),
		Execute: func(context.Context, string, json.RawMessage, func(agent.ToolProgress)) (agent.ToolResult, error) {
			if onExecute != nil {
				onExecute()
			}
			return agent.ToolResult{Content: []llm.ToolResultContent{
				&llm.TextContent{Text: "executed"},
			}}, nil
		},
	}}
}

type countingApprover struct {
	calls int
}

func (a *countingApprover) Decide(context.Context, permission.ApprovalRequest) (permission.ApprovalResponse, error) {
	a.calls++
	return permission.ApprovalResponse{Choice: permission.AllowOnce}, nil
}
