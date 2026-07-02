// Command basic runs one prompt through a stateful agent and prints the final
// assistant message from the transcript.
//
// The agent package wraps the llm package with retained history. Each Prompt
// appends the user message and every assistant/tool message produced by the run
// to the agent transcript.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/agent/basic
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
)

func main() {
	assistant := agent.New(agent.Options{
		SystemPrompt: "You are a concise Go tutor.",
		Model:        llm.GetModel("deepseek", "deepseek-v4-flash"),
	})

	if err := assistant.Prompt(context.Background(), "Explain goroutines in one sentence."); err != nil {
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
