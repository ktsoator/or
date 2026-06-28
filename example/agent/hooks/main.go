// Command hooks demonstrates the agent's interception points: a policy gate that
// blocks a tool (BeforeToolCall), an annotation applied to results
// (AfterToolCall), a model switch between turns across wire protocols
// (PrepareNextTurn), and a turn guard (ShouldStopAfterTurn).
//
// It drafts and runs tools on DeepSeek (OpenAI-compatible), then answers on
// MiniMax (Anthropic-compatible). Set both API keys and run from the repo root:
//
//	export DEEPSEEK_API_KEY=...
//	export MINIMAX_CN_API_KEY=...
//	go run ./example/agent/hooks
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/all" // registers all built-in protocols
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to retrieve weather for,minLength=1"`
}

type notifyArgs struct {
	Message string `json:"message" jsonschema:"description=Notification text,minLength=1"`
}

const maxTurns = 4

func main() {
	tools := []agent.AgentTool{
		{
			Definition: llm.MustTool[weatherArgs]("get_weather", "Get the current weather for a city"),
			Execute: func(_ context.Context, _ string, raw json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				var in weatherArgs
				if err := json.Unmarshal(raw, &in); err != nil {
					return agent.ToolResult{}, err
				}
				text := fmt.Sprintf("It is sunny and 24°C in %s.", strings.TrimSpace(in.City))
				return agent.ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: text}}}, nil
			},
		},
		{
			// This tool is advertised to the model but blocked by the policy gate
			// below, so it never actually runs.
			Definition: llm.MustTool[notifyArgs]("send_notification", "Send a push notification to the user"),
			Execute: func(_ context.Context, _ string, _ json.RawMessage, _ func(agent.ToolResult)) (agent.ToolResult, error) {
				return agent.ToolResult{Content: []llm.ToolResultContent{&llm.TextContent{Text: "sent"}}}, nil
			},
		},
	}

	reviewer := llm.GetModel("minimax-cn", "MiniMax-M3")
	switched := false
	turns := 0

	assistant := agent.New(agent.Options{
		SystemPrompt: "Use get_weather for weather questions. Try send_notification when asked to notify. Be concise.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
		Tools:        tools,

		// BeforeToolCall is a policy gate: it audits every call and blocks one
		// tool. A blocked call becomes an error result the model can react to.
		BeforeToolCall: func(c agent.BeforeToolCallCtx) (block bool, reason string) {
			fmt.Printf("  [gate] %s(%v)\n", c.ToolCall.Name, c.ToolCall.Arguments)
			if c.ToolCall.Name == "send_notification" {
				return true, "notifications are disabled in this demo"
			}
			return false, ""
		},

		// AfterToolCall annotates each executed result for the model.
		AfterToolCall: func(c agent.AfterToolCallCtx) *agent.AfterToolCallResult {
			if c.IsError {
				return nil
			}
			annotated := append(c.Result.Content, &llm.TextContent{Text: "(source: simulated)"})
			return &agent.AfterToolCallResult{Content: annotated}
		},

		// PrepareNextTurn switches to a different model — and a different wire
		// protocol — once, after the tool turn. History is re-adapted automatically.
		PrepareNextTurn: func(agent.TurnCtx) *agent.TurnUpdate {
			if switched {
				return nil
			}
			switched = true
			fmt.Printf("\n  [switch] -> %s/%s (%s)\n", reviewer.Provider, reviewer.ID, reviewer.Protocol)
			return &agent.TurnUpdate{Model: &reviewer}
		},

		// ShouldStopAfterTurn guards against a runaway loop. It is called once per
		// turn, so a counter is enough.
		ShouldStopAfterTurn: func(agent.TurnCtx) bool {
			turns++
			return turns >= maxTurns
		},
	})

	answerStarted := false
	assistant.Subscribe(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.ToolStart:
			fmt.Printf("\n[tool] %s\n", event.ToolName)
		case agent.ToolEnd:
			status := "ok"
			if event.IsError {
				status = "blocked/error"
			}
			fmt.Printf("  [done] %s: %s\n", event.ToolName, status)
		case agent.MessageUpdate:
			if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
				if !answerStarted {
					fmt.Print("\n[answer] ")
					answerStarted = true
				}
				fmt.Print(event.LLMEvent.Delta)
			}
		}
	})

	if err := assistant.Prompt(
		context.Background(),
		"What is the weather in Shanghai? Then notify me about it.",
	); err != nil {
		log.Fatal(err)
	}
	fmt.Println()
}
