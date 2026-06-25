// Command complete demonstrates the smallest non-streaming model request.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ktsoator/or/llm"
)

func main() {
	model := llm.GetModel("deepseek", "deepseek-v4-flash")
	response, err := llm.Complete(
		context.Background(),
		model,
		llm.Prompt("Explain Go channels in three sentences."),
		llm.StreamOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Text())
}
