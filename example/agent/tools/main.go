// Command tools lets an agent run a typed tool loop for a weather question.
//
// Unlike the llm/tools example, this program does not hand-write the loop. The
// agent streams a turn, executes get_weather when the model asks for it, appends
// the tool result, and keeps going until the model gives a final answer.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/agent/tools
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
		SystemPrompt: "Call get_weather before answering any weather or packing question.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
		Tools:        []agent.AgentTool{weather},
	})

	if err := assistant.Prompt(context.Background(), "What should I pack for 3 days in Beijing?"); err != nil {
		log.Fatal(err)
	}

	answer, err := lastAssistant(assistant.Snapshot().Messages)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(answer.Text())
}

func lastAssistant(messages []agent.AgentMessage) (*llm.AssistantMessage, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("transcript is empty")
	}
	message, ok := agent.ToLLM(messages[len(messages)-1])
	if !ok {
		return nil, fmt.Errorf("last transcript message is not an llm message")
	}
	assistant, ok := message.(*llm.AssistantMessage)
	if !ok {
		return nil, fmt.Errorf("last transcript message is %T, want *llm.AssistantMessage", message)
	}
	return assistant, nil
}
