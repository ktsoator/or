// Command stream demonstrates incremental reasoning and text events.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/openai" // registers the OpenAI-compatible protocol
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	events, err := llm.Stream(
		context.Background(),
		model,
		llm.Prompt("Why does a Go channel send synchronize two goroutines?"),
		llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
	)
	if err != nil {
		log.Fatal(err)
	}

	var final *llm.AssistantMessage
	thinkingStarted := false
	answerStarted := false
	for event := range events {
		switch event.Type {
		case llm.EventThinkingDelta:
			if !thinkingStarted {
				fmt.Println("[reasoning]")
				thinkingStarted = true
			}
			fmt.Print(event.Delta)
		case llm.EventTextDelta:
			if !answerStarted {
				if thinkingStarted {
					fmt.Print("\n\n")
				}
				fmt.Println("[answer]")
				answerStarted = true
			}
			fmt.Print(event.Delta)
		case llm.EventDone:
			final = event.Message
		case llm.EventError:
			log.Fatal(event.Err)
		}
	}

	if final == nil {
		log.Fatal("stream closed without a final message")
	}
	fmt.Printf(
		"\n\nstop=%s input=%d output=%d total=%d cost=$%.6f\n",
		final.StopReason,
		final.Usage.Input,
		final.Usage.Output,
		final.Usage.TotalTokens,
		final.Usage.Cost.Total,
	)
}
