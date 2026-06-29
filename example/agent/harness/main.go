// Command harness demonstrates the agent harness: an in-memory session that
// persists the transcript, a second harness that resumes from it, and a system
// prompt rebuilt from TurnInfo before each run.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/agent/harness"
	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

func main() {
	ctx := context.Background()
	session := &harness.InMemorySession{}

	// The system prompt is rebuilt each run and reflects the live transcript.
	buildPrompt := func(info harness.TurnInfo) string {
		return fmt.Sprintf(
			"You are a concise assistant. The transcript currently has %d message(s).",
			len(info.Messages),
		)
	}

	first, err := harness.New(ctx, harness.Options{
		Model:             llm.GetModel("deepseek", "deepseek-v4-flash"),
		Session:           session,
		BuildSystemPrompt: buildPrompt,
	})
	if err != nil {
		log.Fatal(err)
	}
	first.Subscribe(printAssistant)

	fmt.Println("== first harness ==")
	if err := first.Prompt(ctx, "In one sentence, what is an agent harness for?"); err != nil {
		log.Fatal(err)
	}
	printStoredMessages(ctx, session)

	// A fresh harness over the same session resumes the prior transcript, so the
	// next prompt continues the conversation.
	second, err := harness.New(ctx, harness.Options{
		Model:             llm.GetModel("deepseek", "deepseek-v4-flash"),
		Session:           session,
		BuildSystemPrompt: buildPrompt,
	})
	if err != nil {
		log.Fatal(err)
	}
	second.Subscribe(printAssistant)

	fmt.Println("\n== second harness, same session ==")
	fmt.Printf("resumed with %d message(s)\n", len(second.Messages()))
	if err := second.Prompt(ctx, "Add one concrete use case to your previous answer."); err != nil {
		log.Fatal(err)
	}
	printStoredMessages(ctx, session)
}

func printAssistant(event agent.AgentEvent) {
	if event.Type != agent.MessageEnd {
		return
	}
	msg, ok := agent.ToLLM(event.Message)
	if !ok {
		return
	}
	assistant, ok := msg.(*llm.AssistantMessage)
	if !ok {
		return
	}
	if text := assistantText(assistant); text != "" {
		fmt.Println("assistant:", text)
	}
}

func printStoredMessages(ctx context.Context, session *harness.InMemorySession) {
	messages, err := session.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("session now stores %d message(s)\n", len(messages))
}

func assistantText(message *llm.AssistantMessage) string {
	var parts []string
	for _, block := range message.Content {
		if text, ok := block.(*llm.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, " ")
}
