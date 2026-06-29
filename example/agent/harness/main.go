// Command harness demonstrates a file-backed harness session: each run persists
// the transcript to a JSON Lines file, so running the program again resumes the
// prior conversation across processes. It also rebuilds the system prompt from
// TurnInfo before each run. Run it twice to see the resume; delete the printed
// file to start over.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/agent/harness"
	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

func main() {
	ctx := context.Background()
	path := filepath.Join(os.TempDir(), "or-agent-harness-session.jsonl")

	h, err := harness.New(ctx, harness.Options{
		Model:   llm.GetModel("deepseek", "deepseek-v4-flash"),
		Session: harness.NewJSONLSession(path),
		// The system prompt is rebuilt each run and reflects the live transcript.
		BuildSystemPrompt: func(info harness.TurnInfo) string {
			return fmt.Sprintf(
				"You are a concise assistant. The transcript currently has %d message(s).",
				len(info.Messages),
			)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	h.Subscribe(printAssistant)

	// A non-empty transcript means a prior run persisted to the file.
	if resumed := len(h.Messages()); resumed == 0 {
		fmt.Printf("== fresh session (%s) ==\n", path)
		if err := h.Prompt(ctx, "In one sentence, what is an agent harness for?"); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Printf("== resumed %d message(s) from %s ==\n", resumed, path)
		if err := h.Prompt(ctx, "Add one concrete use case to your previous answer."); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("\nsession now stores %d message(s) at %s\n", len(h.Messages()), path)
	fmt.Println("run again to resume, or delete the file to start over.")
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
