// Command harness demonstrates the higher-level agent harness with DeepSeek.
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
	if err := first.Prompt(ctx, "用一句话介绍一下 agent harness 的作用。"); err != nil {
		log.Fatal(err)
	}
	printStoredMessages(ctx, session)

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
	if err := second.Prompt(ctx, "基于刚才的回答，再补充一个使用场景。"); err != nil {
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
	text := assistantText(assistant)
	if text != "" {
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
