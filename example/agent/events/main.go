// Command events prints an agent run as it happens: assistant deltas, tool
// starts, tool progress updates, and tool completion events.
//
// Subscribe before Prompt. The listener receives every event in order on the
// goroutine that drives the run, so real applications should keep the listener
// quick or hand work to another goroutine.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/agent/events
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City name,minLength=1"`
	Days int    `json:"days" jsonschema:"description=Forecast length in days,minimum=1,maximum=10"`
}

func main() {
	weather := agent.AgentTool{
		Definition: llm.MustTool[weatherArgs]("get_weather", "Get a weather forecast for a city"),
		Execute: func(ctx context.Context, callID string, args json.RawMessage, onUpdate func(agent.ToolResult)) (agent.ToolResult, error) {
			var in weatherArgs
			if err := json.Unmarshal(args, &in); err != nil {
				return agent.ToolResult{}, err
			}

			onUpdate(agent.ToolResult{Details: "connecting"})
			onUpdate(agent.ToolResult{Details: "reading forecast"})

			result := fmt.Sprintf("%s: sunny, around 25C for the next %d days", in.City, in.Days)
			return agent.ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: result}},
				Details: map[string]any{
					"city": in.City,
					"days": in.Days,
				},
			}, nil
		},
	}

	assistant := agent.New(agent.Options{
		SystemPrompt:  "Call get_weather before answering any weather or packing question.",
		Model:         llm.GetModel("deepseek", "deepseek-v4-flash"),
		ThinkingLevel: llm.ModelThinkingHigh,
		Tools:         []agent.AgentTool{weather},
	})

	unsubscribe := assistant.Subscribe(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.TurnStart:
			fmt.Println("\n--- turn ---")
		case agent.MessageUpdate:
			if event.LLMEvent == nil {
				return
			}
			switch event.LLMEvent.Type {
			case llm.EventThinkingStart:
				fmt.Print("[thinking] ")
			case llm.EventThinkingDelta:
				fmt.Print(event.LLMEvent.Delta)
			case llm.EventTextStart:
				fmt.Print("\n[answer] ")
			case llm.EventTextDelta:
				fmt.Print(event.LLMEvent.Delta)
			}
		case agent.ToolStart:
			fmt.Printf("\n[tool] %s %v\n", event.ToolName, event.Args)
		case agent.ToolUpdate:
			if result, ok := event.Result.(agent.ToolResult); ok {
				fmt.Printf("[tool update] %v\n", result.Details)
			}
		case agent.ToolEnd:
			fmt.Printf("[tool done] %s error=%v\n", event.ToolName, event.IsError)
		case agent.AgentEnd:
			fmt.Printf("\nappended messages: %d\n", len(event.Messages))
		}
	})
	defer unsubscribe()

	if err := assistant.Prompt(context.Background(), "What should I pack for 3 days in Beijing?"); err != nil {
		log.Fatal(err)
	}
}
