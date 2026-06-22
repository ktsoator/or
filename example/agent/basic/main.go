// Command basic demonstrates the smallest stateful agent: one tool, one prompt.
// It subscribes to the event stream to print the answer as it streams and to
// show when the tool runs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

type weatherArgs struct {
	City string `json:"city" jsonschema:"description=City to retrieve weather for,minLength=1"`
}

func main() {
	weatherTool := agent.AgentTool{
		Definition: llm.MustTool[weatherArgs](
			"get_weather",
			"Get the current weather for a city",
		),
		Execute: func(
			_ context.Context,
			_ string,
			rawArgs json.RawMessage,
			_ func(agent.ToolResult),
		) (agent.ToolResult, error) {
			var args weatherArgs
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return agent.ToolResult{}, fmt.Errorf("decode weather arguments: %w", err)
			}
			// Fixed data keeps the example runnable without a second service.
			text := fmt.Sprintf("It is sunny and 24°C in %s.", strings.TrimSpace(args.City))
			return agent.ToolResult{
				Content: []llm.ToolResultContent{&llm.TextContent{Text: text}},
			}, nil
		},
	}

	assistant := agent.New(agent.Options{
		SystemPrompt: "Call get_weather before answering any weather question. Be concise.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
		Tools:        []agent.AgentTool{weatherTool},
	})

	// The agent runs the whole tool loop; the subscriber only renders events.
	assistant.Subscribe(func(event agent.AgentEvent) {
		switch event.Type {
		case agent.ToolStart:
			fmt.Printf("\n[tool] %s %s\n", event.ToolName, formatArgs(event.Args))
		case agent.MessageUpdate:
			if event.LLMEvent != nil && event.LLMEvent.Type == llm.EventTextDelta {
				fmt.Print(event.LLMEvent.Delta)
			}
		}
	})

	if err := assistant.Prompt(context.Background(), "What is the weather in Shanghai?"); err != nil {
		log.Fatal(err)
	}
	fmt.Println()
}

func formatArgs(args any) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprint(args)
	}
	return string(encoded)
}
