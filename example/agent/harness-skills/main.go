// Command harness-skills demonstrates harness skills and prompt templates:
// registering named instructions, advertising the model-invocable ones in the
// system prompt, invoking a skill explicitly, and running a prompt template.
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

	skills := []harness.Skill{
		{
			Name:        "code-review",
			Description: "Review a code change for correctness and clarity.",
			Content: "Review the code below. Check correctness first, then naming and " +
				"error handling. Be specific and keep it short.",
		},
	}
	templates := []harness.PromptTemplate{
		{
			Name:        "summarize",
			Description: "Summarize a topic in three bullet points.",
			Content:     "Summarize $1 in exactly three short bullet points.",
		},
	}

	h, err := harness.New(ctx, harness.Options{
		Model:           llm.GetModel("deepseek", "deepseek-v4-flash"),
		Skills:          skills,
		PromptTemplates: templates,
		// Advertise the model-invocable skills inside the system prompt so the
		// model knows they exist.
		BuildSystemPrompt: func(info harness.TurnInfo) string {
			base := "You are a concise engineering assistant."
			if listing := harness.FormatSkillsForSystemPrompt(info.Skills); listing != "" {
				return base + "\n\n" + listing
			}
			return base
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	h.Subscribe(printAssistant)

	// Invoke a skill explicitly: its instructions are injected as the turn, with
	// extra context appended.
	fmt.Println("== skill: code-review ==")
	if err := h.Skill(ctx, "code-review",
		"func div(a, b int) int { return a / b }"); err != nil {
		log.Fatal(err)
	}

	// Run a prompt template, substituting its argument.
	fmt.Println("\n== template: summarize ==")
	if err := h.PromptFromTemplate(ctx, "summarize", "goroutines"); err != nil {
		log.Fatal(err)
	}
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

func assistantText(message *llm.AssistantMessage) string {
	var parts []string
	for _, block := range message.Content {
		if text, ok := block.(*llm.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, " ")
}
