// Command conversation carries history across multiple turns.
//
// It shows the step beyond a one-shot Complete: keep the messages in a slice,
// append each reply and follow-up, and send the growing history back every
// turn. The library is stateless, so retaining and resending the history is how
// the model "remembers" earlier turns.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/llm/conversation
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
)

func main() {
	ctx := context.Background()
	model := llm.GetModel("deepseek", "deepseek-v4-flash")

	// The conversation is just a slice of messages the caller owns.
	history := []llm.Message{
		llm.UserText("Name one classic Go concurrency pattern."),
	}

	// Turn 1: ask the first question.
	first := ask(ctx, model, history)
	fmt.Println("A1:", first.Text())

	// Append the reply, then a follow-up that relies on it ("that pattern").
	// Resending the whole history is what lets the model resolve the reference.
	history = append(history, &first)
	history = append(history, llm.UserText("Show a minimal code sketch of that pattern."))

	// Turn 2: the model answers with the earlier turn in context.
	second := ask(ctx, model, history)
	fmt.Println("\nA2:", second.Text())
}

// ask sends the current history with a shared system prompt and returns the
// final assistant message.
func ask(ctx context.Context, model llm.Model, history []llm.Message) llm.AssistantMessage {
	input := llm.Context{
		SystemPrompt: "You are a concise Go tutor. Keep answers short.",
		Messages:     history,
	}

	msg, err := llm.Complete(ctx, model, input, llm.StreamOptions{MaxTokens: 500})
	if err != nil {
		log.Fatal(err)
	}
	return msg
}
