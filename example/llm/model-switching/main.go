// Command model-switching reuses one provider-neutral conversation across two
// models that speak different wire protocols.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"

	_ "github.com/ktsoator/or/llm/all" // registers all built-in protocols
)

func main() {
	ctx := context.Background()
	draftModel := llm.GetModel("deepseek", "deepseek-v4-flash")
	reviewModel := llm.GetModel("minimax-cn", "MiniMax-M3")

	history := []llm.Message{
		llm.UserText("Draft a concise explanation of Go interfaces."),
	}

	draft, err := llm.Complete(
		ctx,
		draftModel,
		llm.Context{Messages: history},
		llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"Draft from %s/%s (%s):\n%s\n\n",
		draftModel.Provider,
		draftModel.ID,
		draftModel.Protocol,
		draft.Text(),
	)

	history = append(history, &draft)
	history = append(history, llm.UserText("Review the explanation for accuracy and improve it."))

	review, err := llm.Complete(
		ctx,
		reviewModel,
		llm.Context{Messages: history},
		llm.StreamOptions{Reasoning: llm.ModelThinkingHigh},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf(
		"Review from %s/%s (%s):\n%s\n",
		reviewModel.Provider,
		reviewModel.ID,
		reviewModel.Protocol,
		review.Text(),
	)
}
