// Command reasoning asks a reasoning-capable model to think before answering and
// streams the reasoning and the final answer as separate phases.
//
// It shows the provider-neutral Reasoning knob: a ModelThinkingLevel that each
// adapter maps to its own native form and clamps to what the model supports.
// Thinking arrives as EventThinking* events, distinct from the EventText* answer,
// so a caller can display or hide it. Non-reasoning models ignore the setting.
//
// The API key is read from the provider's environment variable when
// StreamOptions.APIKey is empty:
//
//	DEEPSEEK_API_KEY=sk-... go run ./example/llm/reasoning
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
	_ "github.com/ktsoator/or/llm/openai" // register the OpenAI-compatible protocol (DeepSeek speaks it)
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")

	events, err := llm.Stream(context.Background(), model,
		llm.Prompt("A farmer must cross a river with a wolf, a goat, and a cabbage. "+
			"The boat carries only one item at a time. How does he get all three across?"),
		llm.StreamOptions{
			Reasoning: llm.ModelThinkingHigh, // off, minimal, low, medium, high, xhigh, max
		})
	if err != nil {
		log.Fatal(err)
	}

	for event := range events {
		switch event.Type {
		case llm.EventThinkingStart:
			fmt.Println("--- thinking ---")
		case llm.EventThinkingDelta:
			fmt.Print(event.Delta)
		case llm.EventTextStart:
			fmt.Println("\n--- answer ---")
		case llm.EventTextDelta:
			fmt.Print(event.Delta)
		case llm.EventDone:
			fmt.Println()
		case llm.EventError:
			log.Fatal(event.Err)
		}
	}
}
